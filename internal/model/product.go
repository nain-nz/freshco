package model

import "time"

// Status produk.
const (
	StatusAvailable      = "available"       // dibuat admin, belum diklaim
	StatusPendingPayment = "pending_payment" // sudah diklaim, DP belum lunas
	StatusActive         = "active"
	StatusSuspended      = "suspended"
)

// Role anggota produk.
const (
	RoleOwner  = "owner"
	RoleMember = "member"
)

// Status tagihan.
const (
	PayPending = "pending"
	PayPaid    = "paid"
	PayExpired = "expired"
	PayFailed  = "failed"
)

type Product struct {
	ID        int        `json:"id"`
	Serial    string     `json:"serial"`
	Status    string     `json:"status"`
	ClaimedAt *time.Time `json:"claimed_at"`
	CreatedAt time.Time  `json:"created_at"`
}

// MyProduct = produk milik user yang sedang login (dipakai app untuk menentukan halaman).
type MyProduct struct {
	ID       int    `json:"id"`
	Serial   string `json:"serial"`
	Status   string `json:"status"`
	Role     string `json:"role"` // owner | member
	HasOrder bool   `json:"has_order"`
}

type Member struct {
	UserID   int       `json:"user_id"`
	Username string    `json:"username"`
	Email    string    `json:"email"`
	Role     string    `json:"role"`
	JoinedAt time.Time `json:"joined_at"`
}

type Order struct {
	ID        int `json:"id"`
	ProductID int `json:"product_id"`
	UserID    int `json:"user_id"`
	Total     int `json:"total"`
	DP        int `json:"dp"`
	Tenor     int `json:"tenor"`
	// SubscriptionFee: langganan per bulan untuk order ini (0 = tanpa langganan).
	SubscriptionFee int       `json:"subscription_fee"`
	CreatedAt       time.Time `json:"created_at"`
}

type Payment struct {
	ID              int        `json:"id"`
	OrderID         int        `json:"order_id"`
	Seq             int        `json:"seq"`    // 0 = DP
	Amount          int        `json:"amount"` // total yang ditagihkan (sudah termasuk Fee)
	Fee             int        `json:"fee"`    // bagian langganan di dalam Amount; 0 untuk DP dan tagihan lama
	DueDate         time.Time  `json:"due_date"`
	Method          *string    `json:"method"`
	ProviderOrderID *string    `json:"provider_order_id,omitempty"`
	QRExpiresAt     *time.Time `json:"qr_expires_at,omitempty"` // QR terakhir; kosong kalau belum pernah dibuat
	Status          string     `json:"status"`
	PaidAt          *time.Time `json:"paid_at"`
}

// AdminProduct = baris di daftar produk admin (dengan pemilik dan ringkasan tagihan).
type AdminProduct struct {
	Product
	OwnerID       *int    `json:"owner_id"`
	OwnerUsername *string `json:"owner_username"`
	OwnerEmail    *string `json:"owner_email"`
	MemberCount   int     `json:"member_count"`
	PaidCount     int     `json:"paid_count"`
	PaymentCount  int     `json:"payment_count"`
}

type AdminProductDetail struct {
	AdminProduct
	Members  []Member  `json:"members"` // owner dulu, lalu anggota; dipakai admin untuk membuka detail user
	Order    *Order    `json:"order"`
	Payments []Payment `json:"payments"`
}
