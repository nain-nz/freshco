package service

import (
	"database/sql"
	"encoding/csv"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"smartfood/internal/model"
)

const (
	defaultLimit = 50
	maxLimit     = 500
	exportLimit  = 100000
)

type ReadingService struct{ db *sql.DB }

func NewReadingService(db *sql.DB) *ReadingService { return &ReadingService{db: db} }

type HistoryResult struct {
	Data   []model.Reading `json:"data"`
	Total  int             `json:"total"`
	Limit  int             `json:"limit"`
	Offset int             `json:"offset"`
}

type StatsResult struct {
	Channel  string            `json:"channel"`
	Interval string            `json:"interval"`
	From     time.Time         `json:"from"`
	To       time.Time         `json:"to"`
	Summary  model.StatSummary `json:"summary"`
	Points   []model.StatPoint `json:"points"`
}

func (s *ReadingService) validateChannel(name string) error {
	if name == "" {
		return nil
	}
	chs, err := s.Channels()
	if err != nil {
		return err
	}
	for _, c := range chs {
		if c.Name == name {
			return nil
		}
	}
	return NewValidationError("channel tidak dikenal: " + name)
}

func (s *ReadingService) Channels() ([]model.Channel, error) {
	rows, err := s.db.Query("SELECT id, device_id, name FROM channels ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []model.Channel{}
	for rows.Next() {
		var c model.Channel
		if err := rows.Scan(&c.ID, &c.DeviceID, &c.Name); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Latest mengambil pembacaan terakhir tiap channel.
func (s *ReadingService) Latest(pid int) ([]model.Reading, error) {
	rows, err := s.db.Query(
		"SELECT r.`time`, c.name, r.`value` "+
			"FROM channels c JOIN readings r ON r.channel_id = c.id AND r.product_id = ? "+
			"AND r.`time` = (SELECT MAX(`time`) FROM readings WHERE product_id = ? AND channel_id = c.id) "+
			"ORDER BY c.id", pid, pid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []model.Reading{}
	for rows.Next() {
		var x model.Reading
		if err := rows.Scan(&x.Time, &x.Channel, &x.Value); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *ReadingService) History(f model.ReadingFilter) (*HistoryResult, error) {
	if err := s.validateChannel(f.Channel); err != nil {
		return nil, err
	}
	if f.Limit <= 0 {
		f.Limit = defaultLimit
	}
	if f.Limit > maxLimit {
		f.Limit = maxLimit
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	data, err := s.readings(f)
	if err != nil {
		return nil, err
	}
	total, err := s.count(f)
	if err != nil {
		return nil, err
	}
	return &HistoryResult{Data: data, Total: total, Limit: f.Limit, Offset: f.Offset}, nil
}

// Export mengambil data untuk CSV (tanpa paging, dibatasi exportLimit baris).
func (s *ReadingService) Export(f model.ReadingFilter) ([]model.Reading, error) {
	if err := s.validateChannel(f.Channel); err != nil {
		return nil, err
	}
	for _, name := range f.Channels {
		if err := s.validateChannel(name); err != nil {
			return nil, err
		}
	}
	f.Limit, f.Offset = exportLimit, 0
	return s.readings(f)
}

func WriteCSV(w io.Writer, rows []model.Reading) error {
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"time", "channel", "value"}); err != nil {
		return err
	}
	for _, r := range rows {
		rec := []string{
			r.Time.Format("2006-01-02 15:04:05.000"),
			r.Channel,
			strconv.FormatFloat(r.Value, 'f', -1, 64),
		}
		if err := cw.Write(rec); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func (s *ReadingService) Stats(pid int, channel, interval string, from, to time.Time) (*StatsResult, error) {
	if channel == "" {
		return nil, NewValidationError("parameter channel wajib diisi")
	}
	if err := s.validateChannel(channel); err != nil {
		return nil, err
	}
	if interval == "" {
		interval = "hour"
	}
	if _, ok := bucketExprs[interval]; !ok {
		return nil, NewValidationError("interval harus 5min, 15min, 30min, hour, 6hour, atau day")
	}
	if to.IsZero() {
		to = time.Now()
	}
	if from.IsZero() {
		if interval == "day" {
			from = to.AddDate(0, 0, -7)
		} else {
			from = to.Add(-24 * time.Hour)
		}
	}
	if !from.Before(to) {
		return nil, NewValidationError("from harus sebelum to")
	}

	points, summary, err := s.stats(pid, channel, interval, from, to)
	if err != nil {
		return nil, err
	}
	return &StatsResult{
		Channel: channel, Interval: interval, From: from, To: to,
		Summary: summary, Points: points,
	}, nil
}

// query database

const baseFrom = " FROM readings r JOIN channels c ON c.id = r.channel_id"

func buildWhere(f model.ReadingFilter) (string, []any) {
	conds := []string{"r.product_id = ?"}
	args := []any{f.ProductID}
	if f.Channel != "" {
		conds = append(conds, "c.name = ?")
		args = append(args, f.Channel)
	}
	if len(f.Channels) > 0 {
		conds = append(conds, "c.name IN "+inClause(len(f.Channels)))
		for _, name := range f.Channels {
			args = append(args, name)
		}
	}
	if !f.From.IsZero() {
		conds = append(conds, "r.`time` >= ?")
		args = append(args, f.From)
	}
	if !f.To.IsZero() {
		conds = append(conds, "r.`time` <= ?")
		args = append(args, f.To)
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

func (s *ReadingService) readings(f model.ReadingFilter) ([]model.Reading, error) {
	where, args := buildWhere(f)
	q := "SELECT r.`time`, c.name, r.`value`" + baseFrom + where +
		" ORDER BY r.`time` DESC, c.id LIMIT ? OFFSET ?"
	args = append(args, f.Limit, f.Offset)

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []model.Reading{}
	for rows.Next() {
		var x model.Reading
		if err := rows.Scan(&x.Time, &x.Channel, &x.Value); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *ReadingService) count(f model.ReadingFilter) (int, error) {
	where, args := buildWhere(f)
	var n int
	err := s.db.QueryRow("SELECT COUNT(*)"+baseFrom+where, args...).Scan(&n)
	return n, err
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

// bucketExprs: ekspresi SQL pengelompokan waktu untuk tiap interval grafik. Semuanya
// pilihan tetap di sini (bukan input user), jadi aman disisipkan ke SQL. Hasilnya
// selalu teks "YYYY-MM-DD HH:MM:SS" awal bucket.
var bucketExprs = map[string]string{
	"5min":  "CONCAT(DATE_FORMAT(r.`time`, '%Y-%m-%d %H:'), LPAD(FLOOR(MINUTE(r.`time`) / 5) * 5, 2, '0'), ':00')",
	"15min": "CONCAT(DATE_FORMAT(r.`time`, '%Y-%m-%d %H:'), LPAD(FLOOR(MINUTE(r.`time`) / 15) * 15, 2, '0'), ':00')",
	"30min": "CONCAT(DATE_FORMAT(r.`time`, '%Y-%m-%d %H:'), LPAD(FLOOR(MINUTE(r.`time`) / 30) * 30, 2, '0'), ':00')",
	"hour":  "DATE_FORMAT(r.`time`, '%Y-%m-%d %H:00:00')",
	"6hour": "CONCAT(DATE_FORMAT(r.`time`, '%Y-%m-%d '), LPAD(FLOOR(HOUR(r.`time`) / 6) * 6, 2, '0'), ':00:00')",
	"day":   "DATE_FORMAT(r.`time`, '%Y-%m-%d 00:00:00')",
}

// stats menghitung rata-rata/min/max per bucket waktu untuk satu channel.
func (s *ReadingService) stats(pid int, channel, interval string, from, to time.Time) ([]model.StatPoint, model.StatSummary, error) {
	rows, err := s.db.Query(
		"SELECT "+bucketExprs[interval]+" AS bucket, "+
			"AVG(r.`value`), MIN(r.`value`), MAX(r.`value`), COUNT(*)"+baseFrom+
			" WHERE r.product_id = ? AND c.name = ? AND r.`time` >= ? AND r.`time` <= ?"+
			" GROUP BY bucket ORDER BY bucket",
		pid, channel, from, to)
	if err != nil {
		return nil, model.StatSummary{}, err
	}
	defer rows.Close()

	points := []model.StatPoint{}
	for rows.Next() {
		var bucket string
		var p model.StatPoint
		if err := rows.Scan(&bucket, &p.Avg, &p.Min, &p.Max, &p.Count); err != nil {
			return nil, model.StatSummary{}, err
		}
		t, err := time.ParseInLocation("2006-01-02 15:04:05", bucket, time.Local)
		if err != nil {
			return nil, model.StatSummary{}, err
		}
		p.Time = t
		p.Avg = round2(p.Avg)
		points = append(points, p)
	}
	if err := rows.Err(); err != nil {
		return nil, model.StatSummary{}, err
	}

	var sum model.StatSummary
	var avg, min, max sql.NullFloat64
	err = s.db.QueryRow(
		"SELECT AVG(r.`value`), MIN(r.`value`), MAX(r.`value`), COUNT(*)"+baseFrom+
			" WHERE r.product_id = ? AND c.name = ? AND r.`time` >= ? AND r.`time` <= ?",
		pid, channel, from, to).Scan(&avg, &min, &max, &sum.Count)
	if err != nil {
		return nil, model.StatSummary{}, err
	}
	sum.Avg, sum.Min, sum.Max = round2(avg.Float64), min.Float64, max.Float64
	return points, sum, nil
}
