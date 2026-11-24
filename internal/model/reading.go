package model

import "time"

type Reading struct {
	Time    time.Time `json:"time"`
	Channel string    `json:"channel"`
	Value   float64   `json:"value"`
}

// ReadingFilter dipakai untuk halaman Riwayat dan export CSV.
type ReadingFilter struct {
	ProductID int      // wajib: data selalu dibatasi ke satu produk
	Channel   string   // kosong = semua channel
	Channels  []string // beberapa channel sekaligus (dipakai export); kosong = tidak membatasi
	From      time.Time
	To        time.Time
	Limit     int
	Offset    int
}

// StatPoint = satu titik grafik (satu jam atau satu hari).
type StatPoint struct {
	Time  time.Time `json:"time"`
	Avg   float64   `json:"avg"`
	Min   float64   `json:"min"`
	Max   float64   `json:"max"`
	Count int       `json:"count"`
}

type StatSummary struct {
	Avg   float64 `json:"avg"`
	Min   float64 `json:"min"`
	Max   float64 `json:"max"`
	Count int     `json:"count"`
}

// TableCell = satu sel tabel Riwayat. Value berisi nilai terakhir yang diketahui pada
// waktu itu (null kalau sensor belum pernah mengirim). Changed = true kalau nilai ini
// memang tercatat pada waktu tersebut (bukan nilai lama yang dibawa dari baris sebelumnya).
type TableCell struct {
	Value   *float64 `json:"value"`
	Changed bool     `json:"changed"`
}

// TableRow = satu baris tabel Riwayat: satu waktu, banyak sensor.
type TableRow struct {
	Time  time.Time            `json:"time"`
	Cells map[string]TableCell `json:"cells"`
}

// TableFilter dipakai untuk tabel Riwayat per waktu.
type TableFilter struct {
	ProductID int
	Channels  []string // kosong = semua sensor
	From      time.Time
	To        time.Time
	Limit     int
	Offset    int
}
