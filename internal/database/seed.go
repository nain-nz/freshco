package database

import (
	"database/sql"
	"log"

	"golang.org/x/crypto/bcrypt"

	"smartfood/internal/config"
)

type device struct {
	ID   int
	Name string
}

type channel struct {
	ID       int
	DeviceID int
	Name     string
}

var devices = []device{
	{1, "Device Sensor"},
	{2, "Device Cooling"},
}

var channels = []channel{
	{1, 1, "suhu_atas"},  // posisi 2 pada grid 3x3
	{2, 1, "suhu_kiri"},  // posisi 4 (dulu suhu_tengah)
	{3, 1, "suhu_bawah"}, // posisi 8 (dekat Peltier)
	{4, 1, "gas"},
	{5, 1, "berat"},
	{6, 2, "peltier"},
	{7, 2, "fan"},
	{8, 1, "suhu_kanan"}, // posisi 6
}

// Seed mengisi data awal. Aman dijalankan berulang:
// devices dan channels diperbarui. Settings dibuat per produk (saat admin menambah produk).
// Akun admin dibuat terpisah (SeedAdmin).
func Seed(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, d := range devices {
		if _, err := tx.Exec(
			"INSERT INTO devices (id, name) VALUES (?, ?) ON DUPLICATE KEY UPDATE name = VALUES(name)",
			d.ID, d.Name); err != nil {
			return err
		}
	}
	for _, c := range channels {
		if _, err := tx.Exec(
			"INSERT INTO channels (id, device_id, name) VALUES (?, ?, ?) "+
				"ON DUPLICATE KEY UPDATE device_id = VALUES(device_id), name = VALUES(name)",
			c.ID, c.DeviceID, c.Name); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	return nil
}

// SeedAdmin membuat akun admin dari ADMIN_EMAIL / ADMIN_PASSWORD (.env).
// Kalau emailnya sudah ada, akun itu dijadikan admin (password tidak diubah).
func SeedAdmin(db *sql.DB, cfg config.Config) error {
	if cfg.AdminEmail == "" || cfg.AdminPassword == "" {
		log.Println("[seed] ADMIN_EMAIL / ADMIN_PASSWORD kosong, akun admin dilewati")
		return nil
	}
	var id int
	err := db.QueryRow("SELECT id FROM users WHERE email = ?", cfg.AdminEmail).Scan(&id)
	if err == nil {
		_, err = db.Exec("UPDATE users SET role = 'admin', email_verified_at = COALESCE(email_verified_at, NOW()) WHERE id = ?", id)
		if err == nil {
			log.Printf("[seed] %s dijadikan admin", cfg.AdminEmail)
		}
		return err
	}
	if err != sql.ErrNoRows {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(cfg.AdminPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = db.Exec(
		"INSERT INTO users (username, email, password, role, email_verified_at) VALUES (?, ?, ?, 'admin', NOW())",
		cfg.AdminUsername, cfg.AdminEmail, string(hash))
	if err == nil {
		log.Printf("[seed] akun admin %s dibuat", cfg.AdminEmail)
	}
	return err
}
