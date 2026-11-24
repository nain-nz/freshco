package service

import (
	"database/sql"
	"errors"
	"log"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"smartfood/internal/model"
)

const tokenTTL = 10 * 365 * 24 * time.Hour

type AuthService struct {
	db     *sql.DB
	otp    *OTPService
	secret []byte
}

func NewAuthService(db *sql.DB, otp *OTPService, secret []byte) *AuthService {
	return &AuthService{db: db, otp: otp, secret: secret}
}

type tokenClaims struct {
	TV int `json:"tv"` // token_version: naik saat password diganti
	jwt.RegisteredClaims
}

func normalizeEmail(e string) string { return strings.ToLower(strings.TrimSpace(e)) }

func validatePassword(pw string) error {
	if len(pw) < 6 {
		return NewValidationError("password minimal 6 karakter")
	}
	if len(pw) > 72 { // batas bcrypt
		return NewValidationError("password maksimal 72 karakter")
	}
	return nil
}

// sendOTP mengirim kode dan hanya mencatat kegagalan (cooldown dianggap wajar).
func (s *AuthService) sendOTP(u *model.User, purpose string) {
	if err := s.otp.Issue(u, purpose); err != nil && !errors.Is(err, ErrOTPCooldown) {
		log.Printf("[otp] gagal mengirim kode (%s) ke %s: %v", purpose, u.Email, err)
	}
}

// Register membuat akun belum terverifikasi lalu mengirim OTP. Belum ada token:
// user harus memverifikasi email dulu.
func (s *AuthService) Register(username, email, password string, profile model.Profile) (*model.User, error) {
	username = strings.TrimSpace(username)
	email = normalizeEmail(email)

	if l := len(username); l < 3 || l > 50 {
		return nil, NewValidationError("username harus 3-50 karakter")
	}
	if a, err := mail.ParseAddress(email); err != nil || a.Address != email || len(email) > 100 {
		return nil, NewValidationError("format email tidak valid")
	}
	if err := validatePassword(password); err != nil {
		return nil, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	// Username yang masih ditahan akun belum terverifikasi dengan email lain
	// (mis. user ganti email karena OTP tidak datang) dibebaskan dulu.
	if err := s.deleteUnverifiedByUsername(username, email); err != nil {
		return nil, err
	}
	u, err := s.findByEmail(email)
	switch {
	case err == nil && u.EmailVerified:
		// akun aktif sudah memakai email ini
		return nil, ErrEmailTaken
	case err == nil:
		// akun lama belum terverifikasi (user menutup halaman OTP lalu daftar lagi):
		// pakai data terbaru dan kirim OTP lagi, jangan ditolak.
		if err := s.updateUnverified(u.ID, username, string(hash), profile); err != nil {
			return nil, err // ErrUsernameTaken kalau username dipakai akun lain
		}
		if u, err = s.findByID(u.ID); err != nil {
			return nil, err
		}
	case errors.Is(err, ErrNotFound):
		u, err = s.createUser(username, email, string(hash), profile)
		if err != nil {
			return nil, err // ErrEmailTaken / ErrUsernameTaken dikenali handler
		}
	default:
		return nil, err
	}
	s.sendOTP(u, PurposeVerify)
	return u, nil
}

// VerifyEmail memeriksa OTP; kalau benar, akun aktif dan user langsung masuk (dapat token).
func (s *AuthService) VerifyEmail(email, code string) (*model.User, string, error) {
	u, err := s.findByEmail(normalizeEmail(email))
	if errors.Is(err, ErrNotFound) {
		return nil, "", ErrOTPInvalid
	}
	if err != nil {
		return nil, "", err
	}
	if u.EmailVerified {
		return nil, "", ErrOTPInvalid
	}
	if err := s.otp.Verify(u.ID, PurposeVerify, code); err != nil {
		return nil, "", err
	}
	if err := s.markVerified(u.ID, time.Now()); err != nil {
		return nil, "", err
	}
	u, err = s.findByID(u.ID)
	if err != nil {
		return nil, "", err
	}
	token, err := s.issueToken(u)
	return u, token, err
}

// ResendOTP selalu terlihat berhasil, baik email terdaftar atau tidak.
func (s *AuthService) ResendOTP(email string) error {
	u, err := s.findByEmail(normalizeEmail(email))
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if !u.EmailVerified {
		s.sendOTP(u, PurposeVerify)
	}
	return nil
}

func (s *AuthService) Login(email, password string) (*model.User, string, error) {
	u, err := s.findByEmail(normalizeEmail(email))
	if errors.Is(err, ErrNotFound) {
		return nil, "", ErrInvalidCredentials
	}
	if err != nil {
		return nil, "", err
	}
	if bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password)) != nil {
		return nil, "", ErrInvalidCredentials
	}
	if !u.EmailVerified {
		s.sendOTP(u, PurposeVerify) // kirim kode baru supaya user bisa langsung memverifikasi
		return nil, "", ErrEmailNotVerified
	}
	token, err := s.issueToken(u)
	return u, token, err
}

// ForgotPassword selalu terlihat berhasil, supaya email orang lain tidak bisa ditebak.
func (s *AuthService) ForgotPassword(email string) error {
	u, err := s.findByEmail(normalizeEmail(email))
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	s.sendOTP(u, PurposeReset)
	return nil
}

func (s *AuthService) ResetPassword(email, code, newPassword string) error {
	if err := validatePassword(newPassword); err != nil {
		return err
	}
	u, err := s.findByEmail(normalizeEmail(email))
	if errors.Is(err, ErrNotFound) {
		return ErrOTPInvalid
	}
	if err != nil {
		return err
	}
	if err := s.otp.Verify(u.ID, PurposeReset, code); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.setPassword(u.ID, string(hash), time.Now())
}

// UpdateUsername mengganti username (3-50 karakter, harus unik).
func (s *AuthService) UpdateUsername(id int, username string) (*model.User, error) {
	username = strings.TrimSpace(username)
	if l := len(username); l < 3 || l > 50 {
		return nil, NewValidationError("username harus 3-50 karakter")
	}
	if _, err := s.db.Exec("UPDATE users SET username = ? WHERE id = ?", username, id); err != nil {
		return nil, mapDuplicate(err)
	}
	return s.findByID(id)
}

// ChangePassword mengganti password setelah password lama dicek. Token lama jadi
// tidak berlaku (token_version naik), jadi token baru dikembalikan supaya user
// yang sedang memakai aplikasi tidak terlempar keluar.
func (s *AuthService) ChangePassword(id int, current, newPassword string) (string, error) {
	if err := validatePassword(newPassword); err != nil {
		return "", err
	}
	u, err := s.findByID(id)
	if err != nil {
		return "", err
	}
	if bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(current)) != nil {
		return "", ErrWrongPassword
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	if err := s.setPassword(id, string(hash), time.Now()); err != nil {
		return "", err
	}
	if u, err = s.findByID(id); err != nil {
		return "", err
	}
	return s.issueToken(u)
}

// UpdateProfile mengganti username dan data diri sekaligus (satu UPDATE, jadi
// berhasil semua atau gagal semua). Tanggal lahir hanya boleh diubah seminggu sekali.
func (s *AuthService) UpdateProfile(id int, username, phone, birthDate, address string) (*model.User, error) {
	username = strings.TrimSpace(username)
	if l := len(username); l < 3 || l > 50 {
		return nil, NewValidationError("username harus 3-50 karakter")
	}
	p, err := ValidateProfile(phone, birthDate, address)
	if err != nil {
		return nil, err
	}
	cur, err := s.findByID(id)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	birthChanged, err := checkBirthDateChange(cur, p.BirthDate, now)
	if err != nil {
		return nil, err
	}
	q := "UPDATE users SET username = ?, phone = ?, birth_date = ?, address = ?"
	args := []any{username, p.Phone, p.BirthDate, p.Address}
	if birthChanged {
		q += ", birth_date_changed_at = ?"
		args = append(args, now)
	}
	q += " WHERE id = ?"
	args = append(args, id)
	if _, err := s.db.Exec(q, args...); err != nil {
		return nil, mapDuplicate(err)
	}
	return s.findByID(id)
}

// UpdateDetails mengganti data diri (nomor HP, tanggal lahir, alamat), username tetap.
// Aturan tanggal lahir sama dengan UpdateProfile.
func (s *AuthService) UpdateDetails(id int, phone, birthDate, address string) (*model.User, error) {
	cur, err := s.findByID(id)
	if err != nil {
		return nil, err
	}
	return s.UpdateProfile(id, cur.Username, phone, birthDate, address)
}

func (s *AuthService) GetUser(id int) (*model.User, error) { return s.findByID(id) }

func (s *AuthService) issueToken(u *model.User) (string, error) {
	now := time.Now()
	claims := tokenClaims{
		TV: u.TokenVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.Itoa(u.ID),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(tokenTTL)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
}

// Authenticate memvalidasi token, lalu memuat user dari database. Token lama
// ditolak setelah password diganti (token_version tidak cocok).
func (s *AuthService) Authenticate(token string) (*model.User, error) {
	claims := &tokenClaims{}
	t, err := jwt.ParseWithClaims(token, claims,
		func(*jwt.Token) (any, error) { return s.secret, nil },
		jwt.WithValidMethods([]string{"HS256"}))
	if err != nil || !t.Valid {
		return nil, errors.New("token tidak valid")
	}
	id, err := strconv.Atoi(claims.Subject)
	if err != nil {
		return nil, errors.New("token tidak valid")
	}
	u, err := s.findByID(id)
	if err != nil {
		return nil, errors.New("pengguna tidak ditemukan")
	}
	if u.TokenVersion != claims.TV || !u.EmailVerified {
		return nil, errors.New("token sudah tidak berlaku")
	}
	return u, nil
}

// query database (tabel users)

const userColumns = "id, username, email, phone, DATE_FORMAT(birth_date, '%Y-%m-%d'), address, " +
	"password, role, email_verified_at, token_version, created_at, birth_date_changed_at"

// createUser membuat akun baru, belum terverifikasi.
func (s *AuthService) createUser(username, email, passwordHash string, p model.Profile) (*model.User, error) {
	res, err := s.db.Exec(
		"INSERT INTO users (username, email, phone, birth_date, address, password) VALUES (?, ?, ?, ?, ?, ?)",
		username, email, p.Phone, p.BirthDate, p.Address, passwordHash)
	if err != nil {
		return nil, mapDuplicate(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.findByID(int(id))
}

// updateUnverified menimpa username dan password akun yang BELUM terverifikasi
func (s *AuthService) updateUnverified(id int, username, passwordHash string, p model.Profile) error {
	_, err := s.db.Exec(
		"UPDATE users SET username = ?, password = ?, phone = ?, birth_date = ?, address = ? "+
			"WHERE id = ? AND email_verified_at IS NULL",
		username, passwordHash, p.Phone, p.BirthDate, p.Address, id)
	return mapDuplicate(err)
}

// deleteUnverifiedByUsername menghapus akun BELUM terverifikasi yang memakai
func (s *AuthService) deleteUnverifiedByUsername(username, exceptEmail string) error {
	_, err := s.db.Exec(
		"DELETE FROM users WHERE username = ? AND email <> ? AND email_verified_at IS NULL",
		username, exceptEmail)
	return err
}

func (s *AuthService) findByEmail(email string) (*model.User, error) {
	return scanUser(s.db.QueryRow("SELECT "+userColumns+" FROM users WHERE email = ?", email))
}

func (s *AuthService) findByID(id int) (*model.User, error) {
	return scanUser(s.db.QueryRow("SELECT "+userColumns+" FROM users WHERE id = ?", id))
}

func (s *AuthService) markVerified(id int, at time.Time) error {
	_, err := s.db.Exec(
		"UPDATE users SET email_verified_at = COALESCE(email_verified_at, ?) WHERE id = ?", at, id)
	return err
}

// setPassword mengganti password, menaikkan token_version (token lama tidak berlaku),
// dan menandai email terverifikasi (OTP reset membuktikan pemilik email).
func (s *AuthService) setPassword(id int, passwordHash string, at time.Time) error {
	_, err := s.db.Exec(
		"UPDATE users SET password = ?, token_version = token_version + 1, "+
			"email_verified_at = COALESCE(email_verified_at, ?) WHERE id = ?",
		passwordHash, at, id)
	return err
}

type rowScanner interface{ Scan(dest ...any) error }

// scanUser membaca kolom [userColumns]. [extra] = kolom tambahan di belakangnya (mis. COUNT).
func scanUser(row rowScanner, extra ...any) (*model.User, error) {
	var u model.User
	var verified, birthChanged sql.NullTime
	var phone, birth, addr sql.NullString
	dest := append([]any{&u.ID, &u.Username, &u.Email, &phone, &birth, &addr, &u.Password, &u.Role,
		&verified, &u.TokenVersion, &u.CreatedAt, &birthChanged}, extra...)
	err := row.Scan(dest...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	u.EmailVerified = verified.Valid
	if phone.Valid {
		u.Phone = &phone.String
	}
	if birth.Valid {
		u.BirthDate = &birth.String
	}
	if addr.Valid {
		u.Address = &addr.String
	}
	if birthChanged.Valid {
		if until := birthChanged.Time.Add(BirthDateCooldown); until.After(time.Now()) {
			u.BirthDateLockedUntil = &until
		}
	}
	return &u, nil
}

// mapDuplicate mengubah error MySQL 1062 (duplicate entry) menjadi error yang jelas.
func mapDuplicate(err error) error {
	var me *mysql.MySQLError
	if errors.As(err, &me) && me.Number == 1062 {
		msg := me.Message
		if i := strings.LastIndex(msg, "for key"); i >= 0 {
			msg = msg[i:] // cek nama key saja, bukan nilai yang duplikat
		}
		switch {
		case strings.Contains(msg, "email"):
			return ErrEmailTaken
		case strings.Contains(msg, "username"):
			return ErrUsernameTaken
		}
	}
	return err
}
