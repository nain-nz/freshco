package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"smartfood/internal/config"
	"smartfood/internal/database"
	"smartfood/internal/mailer"
	"smartfood/internal/mqttsub"
	"smartfood/internal/payment"
	"smartfood/internal/service"
)

func usage() {
	fmt.Println(`Pemakaian: go run . [perintah]

  (tanpa perintah)   sama seperti serve
  migrate            jalankan migration baru (up)
  migrate down [n]   rollback n langkah (default 1)
  migrate fresh      hapus semua tabel lalu migrate ulang (data hilang!)
  migrate version    lihat versi migration saat ini
  migrate force N    paksa versi ke N (untuk memperbaiki status dirty)
  seed               isi data awal + akun admin (aman dijalankan berulang)
  serve              jalankan subscriber MQTT + REST API`)
}

func main() {
	_ = godotenv.Load() // abaikan jika .env tidak ada (pakai env sistem)
	cfg := config.Load()

	// tanpa perintah = jalankan server, jadi cukup `go run .`
	if len(os.Args) < 2 {
		serve(cfg)
		return
	}

	switch os.Args[1] {
	case "migrate":
		if err := database.Migrate(cfg, os.Args[2:]); err != nil {
			log.Fatalf("migrate gagal: %v", err)
		}
	case "seed":
		db, err := database.Open(cfg)
		if err != nil {
			log.Fatalf("koneksi database gagal: %v", err)
		}
		defer db.Close()
		if err := database.Seed(db); err != nil {
			log.Fatalf("seed gagal: %v", err)
		}
		if err := database.SeedAdmin(db, cfg); err != nil {
			log.Fatalf("seed admin gagal: %v", err)
		}
		log.Println("seed selesai")
	case "serve":
		serve(cfg)
	default:
		usage()
		os.Exit(1)
	}
}

func serve(cfg config.Config) {
	if cfg.JWTSecret == "" {
		log.Fatal("JWT_SECRET kosong, isi dulu di file .env")
	}

	db, err := database.Open(cfg)
	if err != nil {
		log.Fatalf("koneksi database gagal: %v", err)
	}
	defer db.Close()

	// MQTT: terima data sensor + kirim perintah ke box
	sub, err := mqttsub.New(cfg, db)
	if err != nil {
		log.Fatalf("subscriber gagal dibuat: %v", err)
	}
	sub.Start()
	defer sub.Stop()

	// email (OTP)
	mail, err := mailer.New(cfg)
	if err != nil {
		log.Fatalf("konfigurasi email salah: %v", err)
	}
	log.Printf("[mail] provider=%s", mail.Name())

	secret := []byte(cfg.JWTSecret)
	otp := service.NewOTPService(db, mail, secret, cfg.MailFromName)
	auth := service.NewAuthService(db, otp, secret)
	products := service.NewProductService(db, cfg)
	pay := service.NewPaymentService(db, cfg,
		payment.NewMidtrans(cfg.MidtransServerKey, cfg.MidtransSandbox).WithNotifyURL(cfg.MidtransNotifyURL))
	if cfg.MidtransNotifyURL != "" {
		log.Printf("[payment] webhook aktif: Midtrans memberi kabar ke %s/api/payments/notify", strings.TrimRight(cfg.MidtransNotifyURL, "/"))
	} else {
		log.Println("[payment] MIDTRANS_NOTIFY_URL kosong: status pembayaran dicek ke Midtrans (polling)")
	}
	if cfg.MidtransServerKey == "" {
		log.Println("[payment] MIDTRANS_SERVER_KEY kosong: QRIS mati, cash tetap jalan")
	}

	// cek cicilan telat: sekali di awal lalu tiap jam
	jobCtx, stopJobs := context.WithCancel(context.Background())
	defer stopJobs()
	go pay.RunOverdueJob(jobCtx)

	// REST API
	engine := newRouter(deps{
		Auth:    auth,
		Product: products,
		Payment: pay,
		Reading: service.NewReadingService(db),
		Alert:   service.NewAlertService(db),
		Setting: service.NewSettingService(db, sub),
	})
	srv := &http.Server{Addr: ":" + cfg.HTTPPort, Handler: engine}
	go func() {
		log.Printf("[http] API berjalan di port %s", cfg.HTTPPort)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http server gagal: %v", err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	log.Println("berhenti...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}
