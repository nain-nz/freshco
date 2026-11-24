package main

import (
	"net/http"
	"strings"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"smartfood/internal/handler"
	"smartfood/internal/middleware"
	"smartfood/internal/service"
)

type deps struct {
	Auth    *service.AuthService
	Product *service.ProductService
	Payment *service.PaymentService
	Reading *service.ReadingService
	Alert   *service.AlertService
	Setting *service.SettingService
}

func newRouter(d deps) *gin.Engine {
	r := gin.Default()

	r.Use(cors.New(cors.Config{
		AllowOriginFunc: func(origin string) bool {
			return strings.HasPrefix(origin, "http://localhost:") ||
				strings.HasPrefix(origin, "http://127.0.0.1:")
		},
		AllowMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders: []string{"Origin", "Content-Type", "Accept", "Authorization"},
	}))

	r.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })

	authH := handler.NewAuthHandler(d.Auth, d.Product)
	prodH := handler.NewProductHandler(d.Product, d.Payment)
	adminH := handler.NewAdminHandler(d.Product, d.Payment, d.Auth)
	readH := handler.NewReadingHandler(d.Reading)
	alertH := handler.NewAlertHandler(d.Alert)
	setH := handler.NewSettingHandler(d.Setting)

	api := r.Group("/api")

	api.POST("/register", authH.Register)
	api.POST("/login", authH.Login)
	api.POST("/verify-email", authH.VerifyEmail)
	api.POST("/resend-otp", authH.ResendOTP)
	api.POST("/forgot-password", authH.ForgotPassword)
	api.POST("/reset-password", authH.ResetPassword)
	api.POST("/payments/notify", adminH.Notify) // webhook Midtrans

	p := api.Group("", middleware.AuthRequired(d.Auth.Authenticate))
	p.GET("/me", authH.Me)
	p.PUT("/me", authH.UpdateProfile)
	p.PUT("/me/details", authH.UpdateDetails) // nomor HP, tanggal lahir, alamat
	p.PUT("/me/profile", authH.UpdateFullProfile) // username + data diri sekaligus
	p.PUT("/password", authH.ChangePassword)
	p.GET("/channels", readH.Channels)
	p.POST("/products/claim", prodH.Claim)
	p.POST("/products/join", prodH.Join)

	// admin
	a := p.Group("/admin", middleware.AdminRequired())
	a.GET("/products", adminH.List)
	a.POST("/products", adminH.Create)
	a.GET("/products/:id", adminH.Get)
	a.DELETE("/products/:id", adminH.Delete)               // hapus produk yang belum diklaim
	a.GET("/users", adminH.Users)                          // tabel pengguna (cari username / email / no. HP)
	a.GET("/users/:id", adminH.UserDetail)                 // detail pengguna + produknya
	a.POST("/products/:id/reset-claim", adminH.ResetClaim) // buat ulang kode klaim (produk yang belum diklaim)
	a.POST("/products/:id/status", adminH.SetStatus)       // ubah status produk (suspend / aktifkan)
	a.POST("/payments/:id/confirm-cash", adminH.ConfirmCash)

	//  per produk: user harus anggota produk itu
	pr := p.Group("/products/:pid", middleware.ProductMember(d.Product.Membership))
	pr.GET("/members", prodH.Members) // daftar anggota produk (owner dan member)
	pr.POST("/leave", prodH.Leave)    // keluar dari kelompok produk

	// khusus owner
	own := pr.Group("", middleware.OwnerOnly())
	own.GET("/billing", prodH.Billing)
	own.POST("/order", prodH.CreateOrder)           // pilih cara bayar (tenor 2/4/6)
	own.POST("/payments/:payid/pay", prodH.Pay)     // minta QRIS untuk satu tagihan
	own.POST("/payments/:payid/check", prodH.Check) // cek status pembayaran
	own.GET("/invite", prodH.ActiveInvite)          // lihat kode undangan
	own.POST("/invite", prodH.CreateInvite)         // buat kode undangan baru
	own.DELETE("/members/:uid", prodH.RemoveMember) // keluarkan anggota dari kelompok

	act := pr.Group("", middleware.ProductActive())
	act.GET("/latest", readH.Latest)                    // nilai semua channel
	act.GET("/readings", readH.History)                 // riwayat bacaan
	act.GET("/readings/table", readH.Table)             // tabel riwayat: satu baris per waktu, sensor jadi kolom
	act.GET("/readings/export", readH.Export)           // unduh riwayat sebagai file CSV
	act.GET("/stats", readH.Stats)                      // agregasi rata-rata/min/max per interval (untuk grafik)
	act.GET("/alerts", alertH.List)                     // daftar alert
	act.GET("/alerts/unread-count", alertH.UnreadCount) // jumlah alert belum dibaca (badge lonceng)
	act.PATCH("/alerts/:id/ack", alertH.Ack)            // tandai satu alert sudah dibaca
	act.POST("/alerts/ack-all", alertH.AckAll)          // tandai semua alert sudah dibaca
	act.POST("/alerts/delete", alertH.Delete)           // hapus beberapa alert sekaligus (body: {"ids":[...]})
	act.GET("/settings", setH.Get)                      // baca pengaturan produk
	act.PUT("/settings", setH.Update)

	return r
}
