package config

import (
	"os"
	"strconv"
)

type Config struct {
	DBHost, DBPort, DBUser, DBPassword, DBName string
	MQTTHost, MQTTPort, MQTTUser, MQTTPassword string
	JWTSecret, HTTPPort                        string

	// email (OTP)
	MailProvider, BrevoAPIKey, MailFromEmail, MailFromName string

	PublicBaseURL string

	// akun admin awal (dibuat oleh: go run . seed)
	AdminUsername, AdminEmail, AdminPassword string

	// harga produk dan pembayaran (rupiah)
	ProductPrice    int // harga total
	DPAmount        int // pembayaran awal
	SubscriptionFee int // biaya langganan per bulan, ditambahkan ke tiap tagihan cicilan
	OverdueDays     int // produk di-suspend kalau cicilan telat lebih dari ini
	InviteTTLHours  int // masa berlaku kode undangan

	// Penyedia QRIS (Midtrans Core API, sandbox). Kosong = QRIS mati, cash tetap jalan.
	MidtransServerKey string
	MidtransSandbox   bool
	MidtransNotifyURL string
}

func Load() Config {
	return Config{
		DBHost:       get("DB_HOST", "127.0.0.1"),
		DBPort:       get("DB_PORT", "3306"),
		DBUser:       get("DB_USER", "root"),
		DBPassword:   get("DB_PASSWORD", ""),
		DBName:       get("DB_NAME", "smartfood"),
		MQTTHost:     get("MQTT_HOST", "127.0.0.1"),
		MQTTPort:     get("MQTT_PORT", "1883"),
		MQTTUser:     get("MQTT_USER", ""),
		MQTTPassword: get("MQTT_PASSWORD", ""),
		JWTSecret:    get("JWT_SECRET", ""),
		HTTPPort:     get("HTTP_PORT", "8080"),

		MailProvider:  get("MAIL_PROVIDER", "console"),
		BrevoAPIKey:   get("BREVO_API_KEY", ""),
		MailFromEmail: get("MAIL_FROM_EMAIL", ""),
		MailFromName:  get("MAIL_FROM_NAME", "FreshCo"),

		PublicBaseURL: get("PUBLIC_BASE_URL", ""),

		AdminUsername: get("ADMIN_USERNAME", "admin"),
		AdminEmail:    get("ADMIN_EMAIL", ""),
		AdminPassword: get("ADMIN_PASSWORD", ""),

		ProductPrice:    getInt("PRODUCT_PRICE", 500000),
		DPAmount:        getInt("DP_AMOUNT", 100000),
		SubscriptionFee: getInt("SUBSCRIPTION_FEE", 15000),
		OverdueDays:     getInt("OVERDUE_DAYS", 7),
		InviteTTLHours:  getInt("INVITE_TTL_HOURS", 24),

		MidtransServerKey: get("MIDTRANS_SERVER_KEY", ""),
		MidtransSandbox:   get("MIDTRANS_SANDBOX", "true") != "false",
		MidtransNotifyURL: get("MIDTRANS_NOTIFY_URL", ""),
	}
}

func get(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
