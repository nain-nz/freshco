package service

import (
	"database/sql"
	"strings"

	"smartfood/internal/model"
)

// sensorChannels: channel yang tampil di tabel Riwayat (peltier dan fan bukan sensor).
var sensorChannels = []string{"suhu_atas", "suhu_kiri", "suhu_kanan", "suhu_bawah", "gas", "berat"}

type TableResult struct {
	Data     []model.TableRow `json:"data"`
	Channels []string         `json:"channels"`
	Total    int              `json:"total"`
	Limit    int              `json:"limit"`
	Offset   int              `json:"offset"`
}

// resolveTableChannels memastikan setiap channel yang diminta adalah sensor yang dikenal.
// Hasilnya selalu berurutan seperti sensorChannels, tanpa duplikat.
func resolveTableChannels(req []string) ([]string, error) {
	if len(req) == 0 {
		return append([]string(nil), sensorChannels...), nil
	}
	want := map[string]bool{}
	for _, name := range req {
		ok := false
		for _, s := range sensorChannels {
			if s == name {
				ok = true
				break
			}
		}
		if !ok {
			return nil, NewValidationError("channel tidak dikenal: " + name)
		}
		want[name] = true
	}
	out := make([]string, 0, len(want))
	for _, s := range sensorChannels {
		if want[s] {
			out = append(out, s)
		}
	}
	return out, nil
}

func inClause(n int) string {
	return "(" + strings.TrimSuffix(strings.Repeat("?,", n), ",") + ")"
}

// Table membentuk tabel Riwayat: satu baris per waktu di mana minimal satu sensor yang
// dipilih tercatat. Sensor yang tidak berubah pada waktu itu tetap diisi nilai terakhir
// yang diketahui (Changed = false), sehingga baris selalu lengkap.
func (s *ReadingService) Table(f model.TableFilter) (*TableResult, error) {
	chans, err := resolveTableChannels(f.Channels)
	if err != nil {
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

	// kondisi umum: produk, sensor terpilih, rentang waktu
	conds := []string{"r.product_id = ?", "c.name IN " + inClause(len(chans))}
	args := []any{f.ProductID}
	for _, c := range chans {
		args = append(args, c)
	}
	if !f.From.IsZero() {
		conds = append(conds, "r.`time` >= ?")
		args = append(args, f.From)
	}
	if !f.To.IsZero() {
		conds = append(conds, "r.`time` <= ?")
		args = append(args, f.To)
	}
	where := " WHERE " + strings.Join(conds, " AND ")

	res := &TableResult{Data: []model.TableRow{}, Channels: chans, Limit: f.Limit, Offset: f.Offset}

	if err := s.db.QueryRow("SELECT COUNT(DISTINCT r.`time`)"+baseFrom+where, args...).Scan(&res.Total); err != nil {
		return nil, err
	}
	if res.Total == 0 || f.Offset >= res.Total {
		return res, nil
	}

	// waktu-waktu untuk halaman ini (terbaru lebih dulu)
	pageArgs := append(append([]any(nil), args...), f.Limit, f.Offset)
	trows, err := s.db.Query("SELECT DISTINCT r.`time`"+baseFrom+where+
		" ORDER BY r.`time` DESC LIMIT ? OFFSET ?", pageArgs...)
	if err != nil {
		return nil, err
	}
	defer trows.Close()
	times := []model.TableRow{}
	for trows.Next() {
		var row model.TableRow
		if err := trows.Scan(&row.Time); err != nil {
			return nil, err
		}
		times = append(times, row)
	}
	if err := trows.Err(); err != nil {
		return nil, err
	}
	if len(times) == 0 {
		return res, nil
	}
	newest, oldest := times[0].Time, times[len(times)-1].Time

	// nilai yang benar-benar tercatat di antara waktu terlama dan terbaru halaman ini
	type key struct {
		t    string
		name string
	}
	recorded := map[key]float64{}
	rargs := []any{f.ProductID}
	for _, c := range chans {
		rargs = append(rargs, c)
	}
	rargs = append(rargs, oldest, newest)
	vrows, err := s.db.Query("SELECT r.`time`, c.name, r.`value`"+baseFrom+
		" WHERE r.product_id = ? AND c.name IN "+inClause(len(chans))+
		" AND r.`time` >= ? AND r.`time` <= ?", rargs...)
	if err != nil {
		return nil, err
	}
	defer vrows.Close()
	for vrows.Next() {
		var t sql.NullTime
		var name string
		var v float64
		if err := vrows.Scan(&t, &name, &v); err != nil {
			return nil, err
		}
		recorded[key{t.Time.Format("2006-01-02 15:04:05.000"), name}] = v
	}
	if err := vrows.Err(); err != nil {
		return nil, err
	}

	// nilai terakhir tiap sensor sebelum waktu terlama (titik awal untuk isi ulang)
	last := map[string]*float64{}
	for _, name := range chans {
		var v float64
		err := s.db.QueryRow("SELECT r.`value`"+baseFrom+
			" WHERE r.product_id = ? AND c.name = ? AND r.`time` < ? ORDER BY r.`time` DESC LIMIT 1",
			f.ProductID, name, oldest).Scan(&v)
		switch {
		case err == sql.ErrNoRows:
			last[name] = nil
		case err != nil:
			return nil, err
		default:
			x := v
			last[name] = &x
		}
	}

	// susun dari yang terlama ke terbaru supaya nilai lama bisa dibawa maju
	for i := len(times) - 1; i >= 0; i-- {
		row := times[i]
		row.Cells = make(map[string]model.TableCell, len(chans))
		stamp := row.Time.Format("2006-01-02 15:04:05.000")
		for _, name := range chans {
			if v, ok := recorded[key{stamp, name}]; ok {
				x := v
				last[name] = &x
				row.Cells[name] = model.TableCell{Value: &x, Changed: true}
			} else {
				row.Cells[name] = model.TableCell{Value: last[name], Changed: false}
			}
		}
		times[i] = row
	}
	res.Data = times
	return res, nil
}
