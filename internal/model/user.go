package model

import "time"

type User struct {
	ID            int       `json:"id"`
	Username      string    `json:"username"`
	Email         string    `json:"email"`
	Phone         *string   `json:"phone"`      // nomor HP (angka, boleh diawali +); null = belum diisi
	BirthDate     *string   `json:"birth_date"` // "YYYY-MM-DD"; null = belum diisi
	Address       *string   `json:"address"`    // null = belum diisi
	// BirthDateLockedUntil: selama masih di masa depan, tanggal lahir belum boleh diubah
	// lagi (seminggu sejak perubahan terakhir). null = boleh diubah.
	BirthDateLockedUntil *time.Time `json:"birth_date_locked_until"`
	Password      string    `json:"-"`          // hash bcrypt, tidak pernah dikirim ke client
	Role          string    `json:"role"`       // "user" atau "admin" (role owner/member ada di product_members)
	EmailVerified bool      `json:"email_verified"`
	TokenVersion  int       `json:"-"` // dinaikkan saat password diganti, mencabut token lama
	CreatedAt     time.Time `json:"created_at"`
}

// Profile = data diri yang diisi user (sudah divalidasi dan dinormalkan).
type Profile struct {
	Phone     string
	BirthDate string // YYYY-MM-DD
	Address   string
}

// UserProduct = satu produk milik/diikuti user, dipakai di detail user (admin).
type UserProduct struct {
	ID     int    `json:"id"`
	Serial string `json:"serial"`
	Status string `json:"status"`
	Role   string `json:"role"` // owner | member
}

// AdminUser = baris di tabel pengguna admin.
type AdminUser struct {
	User
	ProductCount int `json:"product_count"`
}

type AdminUserDetail struct {
	AdminUser
	Products []UserProduct `json:"products"`
}
