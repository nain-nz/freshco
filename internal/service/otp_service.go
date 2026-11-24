package service

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"smartfood/internal/mailer"
	"smartfood/internal/model"
)

const (
	PurposeVerify = "verify_email"
	PurposeReset  = "reset_password"

	otpTTL         = 10 * time.Minute
	otpMaxAttempts = 5
	otpCooldown    = 60 * time.Second
)

// OTPService membuat, mengirim, dan memverifikasi kode 6 digit.
// Yang disimpan di database hanya hash (HMAC) dari kode, bukan kodenya.
type OTPService struct {
	db      *sql.DB
	mail    mailer.Mailer
	secret  []byte
	appName string
}

func NewOTPService(db *sql.DB, mail mailer.Mailer, secret []byte, appName string) *OTPService {
	return &OTPService{db: db, mail: mail, secret: secret, appName: appName}
}

func (s *OTPService) hash(userID int, purpose, code string) string {
	m := hmac.New(sha256.New, s.secret)
	fmt.Fprintf(m, "%d:%s:%s", userID, purpose, code)
	return hex.EncodeToString(m.Sum(nil))
}

// Issue membuat kode baru (kode lama dibatalkan) lalu mengirimkannya lewat email.
// Mengembalikan ErrOTPCooldown kalau kode terakhir baru dikirim kurang dari 60 detik lalu.
func (s *OTPService) Issue(u *model.User, purpose string) error {
	last, err := s.lastCreatedAt(u.ID, purpose)
	if err != nil {
		return err
	}
	if !last.IsZero() && time.Since(last) < otpCooldown {
		return ErrOTPCooldown
	}

	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return err
	}
	code := fmt.Sprintf("%06d", n.Int64())

	now := time.Now()
	if err := s.invalidateActive(u.ID, purpose, now); err != nil {
		return err
	}
	if err := s.insert(u.ID, purpose, s.hash(u.ID, purpose, code), now.Add(otpTTL), now); err != nil {
		return err
	}
	return s.mail.Send(s.message(u.Email, purpose, code))
}

// Verify memeriksa kode. Semua kegagalan memakai satu pesan yang sama (ErrOTPInvalid).
func (s *OTPService) Verify(userID int, purpose, code string) error {
	code = strings.TrimSpace(code)
	o, err := s.latest(userID, purpose)
	if errors.Is(err, ErrNotFound) {
		return ErrOTPInvalid
	}
	if err != nil {
		return err
	}
	if o.UsedAt != nil || time.Now().After(o.ExpiresAt) || o.Attempts >= otpMaxAttempts {
		return ErrOTPInvalid
	}
	if !hmac.Equal([]byte(o.CodeHash), []byte(s.hash(userID, purpose, code))) {
		_ = s.incrementAttempts(o.ID)
		return ErrOTPInvalid
	}
	return s.markUsed(o.ID, time.Now())
}

func (s *OTPService) message(to, purpose, code string) mailer.Message {
	subject := "Kode verifikasi " + s.appName
	intro := "Gunakan kode berikut untuk memverifikasi email akunmu."
	if purpose == PurposeReset {
		subject = "Kode reset password " + s.appName
		intro = "Gunakan kode berikut untuk mengatur ulang password akunmu."
	}
	minutes := int(otpTTL.Minutes())

	text := fmt.Sprintf("%s\n\nKode: %s\n\nKode berlaku %d menit. Jangan bagikan kode ini kepada siapa pun.",
		intro, code, minutes)
	body := fmt.Sprintf(`<div style="font-family:Arial,sans-serif;max-width:420px;margin:auto;padding:24px">`+
		`<h2 style="color:#0369A1;margin:0 0 12px">%s</h2>`+
		`<p style="color:#334155;margin:0 0 20px">%s</p>`+
		`<div style="background:#F0F9FF;border:1px solid #BAE6FD;border-radius:12px;padding:18px;text-align:center">`+
		`<span style="display:inline-block;font-family:'Courier New',Courier,monospace;font-size:34px;`+
		`font-weight:bold;letter-spacing:10px;color:#0369A1;user-select:all;-webkit-user-select:all">%s</span>`+
		`</div>`+
		`<p style="color:#64748B;font-size:13px;margin:16px 0 0">Kode berlaku %d menit. Jangan bagikan kode ini kepada siapa pun. `+
		`Abaikan email ini jika kamu tidak memintanya.</p></div>`,
		s.appName, intro, code, minutes)

	return mailer.Message{To: to, Subject: subject, Text: text, HTML: body}
}

// query database

// lastCreatedAt = waktu OTP terakhir dibuat (zero kalau belum pernah).
func (s *OTPService) lastCreatedAt(userID int, purpose string) (time.Time, error) {
	var t sql.NullTime
	err := s.db.QueryRow(
		"SELECT MAX(created_at) FROM otp_codes WHERE user_id = ? AND purpose = ?",
		userID, purpose).Scan(&t)
	if err != nil {
		return time.Time{}, err
	}
	return t.Time, nil
}

// invalidateActive menandai semua kode yang masih aktif sebagai terpakai.
func (s *OTPService) invalidateActive(userID int, purpose string, at time.Time) error {
	_, err := s.db.Exec(
		"UPDATE otp_codes SET used_at = ? WHERE user_id = ? AND purpose = ? AND used_at IS NULL",
		at, userID, purpose)
	return err
}

func (s *OTPService) insert(userID int, purpose, codeHash string, expiresAt, createdAt time.Time) error {
	_, err := s.db.Exec(
		"INSERT INTO otp_codes (user_id, purpose, code_hash, expires_at, created_at) VALUES (?, ?, ?, ?, ?)",
		userID, purpose, codeHash, expiresAt, createdAt)
	return err
}

func (s *OTPService) latest(userID int, purpose string) (*model.OTP, error) {
	var o model.OTP
	var used sql.NullTime
	err := s.db.QueryRow(
		"SELECT id, code_hash, attempts, expires_at, used_at FROM otp_codes "+
			"WHERE user_id = ? AND purpose = ? ORDER BY id DESC LIMIT 1",
		userID, purpose).Scan(&o.ID, &o.CodeHash, &o.Attempts, &o.ExpiresAt, &used)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if used.Valid {
		t := used.Time
		o.UsedAt = &t
	}
	return &o, nil
}

func (s *OTPService) incrementAttempts(id int) error {
	_, err := s.db.Exec("UPDATE otp_codes SET attempts = attempts + 1 WHERE id = ?", id)
	return err
}

func (s *OTPService) markUsed(id int, at time.Time) error {
	_, err := s.db.Exec("UPDATE otp_codes SET used_at = ? WHERE id = ?", at, id)
	return err
}
