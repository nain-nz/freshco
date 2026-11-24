package service

import (
	"database/sql"
	"strings"

	"smartfood/internal/model"
)

type AlertService struct{ db *sql.DB }

func NewAlertService(db *sql.DB) *AlertService { return &AlertService{db: db} }

func (s *AlertService) List(pid, limit int, onlyUnread bool) ([]model.Alert, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	q := "SELECT a.id, a.`time`, COALESCE(c.name, ''), a.type, a.`value`, a.acknowledged " +
		"FROM alerts a LEFT JOIN channels c ON c.id = a.channel_id WHERE a.product_id = ?"
	if onlyUnread {
		q += " AND a.acknowledged = FALSE"
	}
	q += " ORDER BY a.`time` DESC, a.id DESC LIMIT ?"

	rows, err := s.db.Query(q, pid, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []model.Alert{}
	for rows.Next() {
		var a model.Alert
		var val sql.NullFloat64
		if err := rows.Scan(&a.ID, &a.Time, &a.Channel, &a.Type, &val, &a.Acknowledged); err != nil {
			return nil, err
		}
		if val.Valid {
			v := val.Float64
			a.Value = &v
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *AlertService) UnreadCount(pid int) (int, error) {
	var n int
	err := s.db.QueryRow("SELECT COUNT(*) FROM alerts WHERE product_id = ? AND acknowledged = FALSE", pid).Scan(&n)
	return n, err
}

func (s *AlertService) Acknowledge(pid, id int) error {
	res, err := s.db.Exec("UPDATE alerts SET acknowledged = TRUE WHERE id = ? AND product_id = ?", id, pid)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		var exists int
		if err := s.db.QueryRow("SELECT COUNT(*) FROM alerts WHERE id = ? AND product_id = ?", id, pid).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			return ErrNotFound
		}
	}
	return nil
}

func (s *AlertService) AcknowledgeAll(pid int) error {
	_, err := s.db.Exec("UPDATE alerts SET acknowledged = TRUE WHERE product_id = ? AND acknowledged = FALSE", pid)
	return err
}

// maxDeleteBatch membatasi jumlah id per permintaan hapus.
const maxDeleteBatch = 200

// DeleteMany menghapus alert milik produk ini. Id milik produk lain diabaikan
// (klausa product_id), jadi satu produk tidak bisa menghapus alert produk lain.
// Mengembalikan jumlah baris yang benar-benar terhapus.
func (s *AlertService) DeleteMany(pid int, ids []int) (int64, error) {
	if len(ids) == 0 {
		return 0, NewValidationError("pilih minimal satu alert")
	}
	if len(ids) > maxDeleteBatch {
		return 0, NewValidationError("terlalu banyak alert dalam satu permintaan")
	}
	marks := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids)+1)
	args = append(args, pid)
	for _, id := range ids {
		args = append(args, id)
	}
	res, err := s.db.Exec("DELETE FROM alerts WHERE product_id = ? AND id IN ("+marks+")", args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
