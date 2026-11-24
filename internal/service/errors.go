package service

import "errors"

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

func NewValidationError(msg string) *ValidationError { return &ValidationError{Message: msg} }

var (
	ErrInvalidCredentials = errors.New("email atau password salah")
	ErrEmailTaken         = errors.New("email sudah terdaftar")
	ErrUsernameTaken      = errors.New("username sudah dipakai")
	ErrNotFound           = errors.New("data tidak ditemukan")
	ErrUnavailable        = errors.New("broker MQTT tidak terhubung")

	ErrEmailNotVerified = errors.New("email belum diverifikasi")
	ErrOTPInvalid       = errors.New("kode salah atau sudah kedaluwarsa")
	ErrOTPCooldown      = errors.New("tunggu sebentar sebelum meminta kode baru")
	ErrWrongPassword    = errors.New("password saat ini salah")

	ErrForbidden     = errors.New("akses ditolak")
	ErrProductLocked = errors.New("produk terkunci, selesaikan pembayaran dulu")
	ErrCodeInvalid   = errors.New("kode tidak valid atau sudah dipakai")
	ErrHasProduct    = errors.New("akun ini sudah punya produk")
	ErrTooMany       = errors.New("terlalu banyak percobaan, coba lagi sebentar")
)
