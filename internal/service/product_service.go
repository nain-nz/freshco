package service

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"

	"smartfood/internal/config"
	"smartfood/internal/model"
)

type ProductService struct {
	db      *sql.DB
	cfg     config.Config
	limiter *attemptLimiter
}

func NewProductService(db *sql.DB, cfg config.Config) *ProductService {
	return &ProductService{db: db, cfg: cfg, limiter: newAttemptLimiter(8, 10*time.Minute)}
}

func isDuplicate(err error) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && me.Number == 1062
}

// ======================= admin =======================

// Create membuat produk baru: serial + kode klaim + 11 pengaturan bawaan.
// Kode klaim hanya dikembalikan di sini (di database hanya hash-nya).
func (s *ProductService) Create() (*model.Product, string, error) {
	for try := 0; try < 5; try++ {
		sn, err := randomCode(6)
		if err != nil {
			return nil, "", err
		}
		serial := "FC-" + sn
		raw, err := randomCode(claimCodeLen)
		if err != nil {
			return nil, "", err
		}
		p, err := s.insertProduct(serial, hashCode(raw))
		if isDuplicate(err) {
			continue // serial atau hash bentrok (sangat jarang), coba lagi
		}
		if err != nil {
			return nil, "", err
		}
		return p, formatCode(raw), nil
	}
	return nil, "", errors.New("gagal membuat serial unik")
}

func (s *ProductService) insertProduct(serial, claimHash string) (*model.Product, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	now := time.Now()
	res, err := tx.Exec(
		"INSERT INTO products (serial, claim_hash, status, created_at) VALUES (?, ?, 'available', ?)",
		serial, claimHash, now)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	for _, kv := range model.DefaultSettings {
		if _, err := tx.Exec(
			"INSERT INTO settings (product_id, `key`, `value`) VALUES (?, ?, ?)", id, kv[0], kv[1]); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &model.Product{ID: int(id), Serial: serial, Status: model.StatusAvailable, CreatedAt: now}, nil
}

// ResetClaimCode membuat kode klaim baru. Hanya untuk produk yang belum diklaim.
func (s *ProductService) ResetClaimCode(id int) (string, error) {
	raw, err := randomCode(claimCodeLen)
	if err != nil {
		return "", err
	}
	res, err := s.db.Exec(
		"UPDATE products SET claim_hash = ? WHERE id = ? AND status = 'available'", hashCode(raw), id)
	if err != nil {
		return "", err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		var cnt int
		if err := s.db.QueryRow("SELECT COUNT(*) FROM products WHERE id = ?", id).Scan(&cnt); err != nil {
			return "", err
		}
		if cnt == 0 {
			return "", ErrNotFound
		}
		return "", NewValidationError("produk sudah diklaim, kode klaim tidak bisa direset")
	}
	return formatCode(raw), nil
}

// SetStatus: admin menyuspend atau mengaktifkan produk yang sudah diklaim.
func (s *ProductService) SetStatus(id int, status string) error {
	if status != model.StatusActive && status != model.StatusSuspended {
		return NewValidationError("status harus active atau suspended")
	}
	res, err := s.db.Exec(
		"UPDATE products SET status = ? WHERE id = ? AND claimed_at IS NOT NULL", status, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		var st string
		err := s.db.QueryRow("SELECT status FROM products WHERE id = ? AND claimed_at IS NOT NULL", id).Scan(&st)
		if errors.Is(err, sql.ErrNoRows) {
			return NewValidationError("produk belum diklaim atau tidak ditemukan")
		}
		return err // sudah berstatus sama: tidak dianggap error
	}
	return nil
}

// Delete menghapus produk yang BELUM diklaim (salah buat / stiker batal dipakai).
// Produk yang sudah dimiliki tidak bisa dihapus.
func (s *ProductService) Delete(id int) error {
	res, err := s.db.Exec("DELETE FROM products WHERE id = ? AND status = 'available'", id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	var cnt int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM products WHERE id = ?", id).Scan(&cnt); err != nil {
		return err
	}
	if cnt == 0 {
		return ErrNotFound
	}
	return NewValidationError("produk sudah dimiliki, tidak bisa dihapus")
}

const adminSelect = `
SELECT p.id, p.serial, p.status, p.claimed_at, p.created_at,
       u.id, u.username, u.email,
       (SELECT COUNT(*) FROM product_members m WHERE m.product_id = p.id),
       COALESCE((SELECT COUNT(*) FROM payments y WHERE y.order_id = o.id AND y.status = 'paid'), 0),
       COALESCE((SELECT COUNT(*) FROM payments y WHERE y.order_id = o.id), 0)
FROM products p
LEFT JOIN product_members pm ON pm.product_id = p.id AND pm.role = 'owner'
LEFT JOIN users u ON u.id = pm.user_id
LEFT JOIN orders o ON o.product_id = p.id `

func (s *ProductService) adminQuery(where string, args ...any) ([]model.AdminProduct, error) {
	rows, err := s.db.Query(adminSelect+where+" ORDER BY p.id DESC LIMIT 200", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []model.AdminProduct{}
	for rows.Next() {
		var a model.AdminProduct
		var claimed sql.NullTime
		var oid sql.NullInt64
		var oname, omail sql.NullString
		if err := rows.Scan(&a.ID, &a.Serial, &a.Status, &claimed, &a.CreatedAt,
			&oid, &oname, &omail, &a.MemberCount, &a.PaidCount, &a.PaymentCount); err != nil {
			return nil, err
		}
		if claimed.Valid {
			t := claimed.Time
			a.ClaimedAt = &t
		}
		if oid.Valid {
			id := int(oid.Int64)
			a.OwnerID = &id
			a.OwnerUsername = &oname.String
			a.OwnerEmail = &omail.String
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// AdminList: semua produk, bisa dicari lewat serial / username / email pemilik.
func (s *ProductService) AdminList(q string) ([]model.AdminProduct, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return s.adminQuery("")
	}
	like := "%" + q + "%"
	return s.adminQuery("WHERE p.serial LIKE ? OR u.username LIKE ? OR u.email LIKE ?", like, like, like)
}

func (s *ProductService) AdminGet(id int) (*model.AdminProduct, error) {
	list, err := s.adminQuery("WHERE p.id = ?", id)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, ErrNotFound
	}
	return &list[0], nil
}

// ======================= keanggotaan =======================

// Membership mengembalikan role user di produk dan status produk.
// ErrForbidden kalau user bukan anggota produk itu.
func (s *ProductService) Membership(userID, pid int) (role, status string, err error) {
	err = s.db.QueryRow(
		"SELECT pm.role, p.status FROM product_members pm JOIN products p ON p.id = pm.product_id "+
			"WHERE pm.user_id = ? AND pm.product_id = ?", userID, pid).Scan(&role, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", ErrForbidden
	}
	return role, status, err
}

// MyProduct = produk user (nil kalau belum punya). Dipakai /api/me.
func (s *ProductService) MyProduct(userID int) (*model.MyProduct, error) {
	var mp model.MyProduct
	err := s.db.QueryRow(
		"SELECT p.id, p.serial, p.status, pm.role, "+
			"EXISTS(SELECT 1 FROM orders o WHERE o.product_id = p.id) "+
			"FROM product_members pm JOIN products p ON p.id = pm.product_id "+
			"WHERE pm.user_id = ? ORDER BY pm.joined_at LIMIT 1", userID).
		Scan(&mp.ID, &mp.Serial, &mp.Status, &mp.Role, &mp.HasOrder)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &mp, nil
}

func (s *ProductService) hasProduct(q interface {
	QueryRow(string, ...any) *sql.Row
}, userID int) (bool, error) {
	var n int
	err := q.QueryRow("SELECT COUNT(*) FROM product_members WHERE user_id = ?", userID).Scan(&n)
	return n > 0, err
}

// Claim: user memasukkan kode klaim dari stiker box dan menjadi owner.
// Satu user hanya boleh punya satu produk (aturan layanan, bukan constraint database).
func (s *ProductService) Claim(userID int, code string) (*model.MyProduct, error) {
	if !s.limiter.Allow(userID) {
		return nil, ErrTooMany
	}
	norm := normalizeCode(code)
	if len(norm) != claimCodeLen {
		return nil, ErrCodeInvalid
	}
	if has, err := s.hasProduct(s.db, userID); err != nil {
		return nil, err
	} else if has {
		return nil, ErrHasProduct
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var pid int
	err = tx.QueryRow(
		"SELECT id FROM products WHERE claim_hash = ? AND status = 'available' FOR UPDATE",
		hashCode(norm)).Scan(&pid)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCodeInvalid
	}
	if err != nil {
		return nil, err
	}
	now := time.Now()
	if _, err := tx.Exec(
		"INSERT INTO product_members (product_id, user_id, role, joined_at) VALUES (?, ?, 'owner', ?)",
		pid, userID, now); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(
		"UPDATE products SET status = 'pending_payment', claimed_at = ? WHERE id = ?", now, pid); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.MyProduct(userID)
}

// ======================= kelompok =======================

// CreateInvite membuat kode undangan baru. Hanya satu kode aktif per produk:
// kode lama yang belum dipakai dihapus. Kode asli hanya dikembalikan di sini.
func (s *ProductService) CreateInvite(pid int) (string, time.Time, error) {
	raw, err := randomCode(inviteCodeLen)
	if err != nil {
		return "", time.Time{}, err
	}
	now := time.Now()
	exp := now.Add(time.Duration(s.cfg.InviteTTLHours) * time.Hour)

	tx, err := s.db.Begin()
	if err != nil {
		return "", time.Time{}, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("DELETE FROM product_invites WHERE product_id = ? AND used_by IS NULL", pid); err != nil {
		return "", time.Time{}, err
	}
	if _, err := tx.Exec(
		"INSERT INTO product_invites (product_id, code_hash, expires_at, created_at) VALUES (?, ?, ?, ?)",
		pid, hashCode(raw), exp, now); err != nil {
		return "", time.Time{}, err
	}
	if err := tx.Commit(); err != nil {
		return "", time.Time{}, err
	}
	return formatCode(raw), exp, nil
}

// ActiveInviteExpiry = kapan kode undangan aktif habis (nil kalau tidak ada).
// Kodenya sendiri tidak bisa ditampilkan lagi (hanya hash yang disimpan).
func (s *ProductService) ActiveInviteExpiry(pid int) (*time.Time, error) {
	var exp time.Time
	err := s.db.QueryRow(
		"SELECT expires_at FROM product_invites WHERE product_id = ? AND used_by IS NULL AND expires_at > ? "+
			"ORDER BY id DESC LIMIT 1", pid, time.Now()).Scan(&exp)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &exp, nil
}

// Join: user memasukkan kode undangan dan menjadi member.
func (s *ProductService) Join(userID int, code string) (*model.MyProduct, error) {
	if !s.limiter.Allow(userID) {
		return nil, ErrTooMany
	}
	norm := normalizeCode(code)
	if len(norm) != inviteCodeLen {
		return nil, ErrCodeInvalid
	}
	if has, err := s.hasProduct(s.db, userID); err != nil {
		return nil, err
	} else if has {
		return nil, ErrHasProduct
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var inviteID, pid int
	err = tx.QueryRow(
		"SELECT id, product_id FROM product_invites WHERE code_hash = ? AND used_by IS NULL AND expires_at > ? FOR UPDATE",
		hashCode(norm), time.Now()).Scan(&inviteID, &pid)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCodeInvalid
	}
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(
		"INSERT INTO product_members (product_id, user_id, role, joined_at) VALUES (?, ?, 'member', ?)",
		pid, userID, time.Now()); err != nil {
		return nil, err
	}
	if _, err := tx.Exec("UPDATE product_invites SET used_by = ? WHERE id = ?", userID, inviteID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.MyProduct(userID)
}

func (s *ProductService) Members(pid int) ([]model.Member, error) {
	rows, err := s.db.Query(
		"SELECT u.id, u.username, u.email, pm.role, pm.joined_at "+
			"FROM product_members pm JOIN users u ON u.id = pm.user_id "+
			"WHERE pm.product_id = ? ORDER BY pm.role = 'owner' DESC, pm.joined_at", pid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Member{}
	for rows.Next() {
		var m model.Member
		if err := rows.Scan(&m.UserID, &m.Username, &m.Email, &m.Role, &m.JoinedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// RemoveMember: owner mengeluarkan member (owner sendiri tidak bisa dikeluarkan).
func (s *ProductService) RemoveMember(pid, userID int) error {
	res, err := s.db.Exec(
		"DELETE FROM product_members WHERE product_id = ? AND user_id = ? AND role = 'member'", pid, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
