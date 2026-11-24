package service

import (
	"strings"

	"smartfood/internal/model"
)

// Kolom user dengan alias u (sama urutannya dengan userColumns).
const adminUserColumns = "u.id, u.username, u.email, u.phone, DATE_FORMAT(u.birth_date, '%Y-%m-%d'), u.address, " +
	"u.password, u.role, u.email_verified_at, u.token_version, u.created_at, u.birth_date_changed_at"

const adminUserSelect = "SELECT " + adminUserColumns + ", " +
	"(SELECT COUNT(*) FROM product_members pm WHERE pm.user_id = u.id) FROM users u " +
	"WHERE u.role = 'user' AND u.email_verified_at IS NOT NULL"

// AdminUsers: daftar pengguna (bukan admin, email terverifikasi). q mencari username, email, atau nomor HP.
func (s *AuthService) AdminUsers(q string) ([]model.AdminUser, error) {
	query := adminUserSelect
	var args []any
	if q = strings.TrimSpace(q); q != "" {
		like := "%" + q + "%"
		phoneLike := "%" + stripPhone(q) + "%"
		query += " AND (u.username LIKE ? OR u.email LIKE ? OR u.phone LIKE ?)"
		args = append(args, like, like, phoneLike)
	}
	rows, err := s.db.Query(query+" ORDER BY u.created_at DESC, u.id DESC LIMIT 200", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []model.AdminUser{}
	for rows.Next() {
		var n int
		u, err := scanUser(rows, &n)
		if err != nil {
			return nil, err
		}
		out = append(out, model.AdminUser{User: *u, ProductCount: n})
	}
	return out, rows.Err()
}

// AdminUserDetail: satu pengguna beserta produk yang dimiliki / diikutinya.
func (s *AuthService) AdminUserDetail(id int) (*model.AdminUserDetail, error) {
	var n int
	u, err := scanUser(s.db.QueryRow(adminUserSelect+" AND u.id = ?", id), &n)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		"SELECT p.id, p.serial, p.status, pm.role FROM product_members pm "+
			"JOIN products p ON p.id = pm.product_id WHERE pm.user_id = ? "+
			"ORDER BY pm.role = 'owner' DESC, pm.joined_at", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	d := &model.AdminUserDetail{AdminUser: model.AdminUser{User: *u, ProductCount: n}, Products: []model.UserProduct{}}
	for rows.Next() {
		var p model.UserProduct
		if err := rows.Scan(&p.ID, &p.Serial, &p.Status, &p.Role); err != nil {
			return nil, err
		}
		d.Products = append(d.Products, p)
	}
	return d, rows.Err()
}
