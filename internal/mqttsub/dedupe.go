package mqttsub

import (
	"math"
	"sync"
)

// changeTolerance adalah selisih minimal terhadap nilai terakhir yang TERSIMPAN
// di database. Selisih lebih kecil dianggap "tidak berubah" dan tidak ditulis.
// Channel yang tidak terdaftar: setiap perbedaan nilai dianggap perubahan.
var changeTolerance = map[string]float64{
	"suhu_atas":  0.1, // DS18B20 beresolusi ~0.06-0.1 C
	"suhu_kiri":  0.1,
	"suhu_kanan": 0.1,
	"suhu_bawah": 0.1,
	"gas":        5,   // ppm, MQ-135 cukup berisik
	"berat":      2,   // gram
	"peltier":    0.5, // 0/1
	"fan":        0.5, // 0/1
}

// filterKey: nilai terakhir diingat per (produk, channel).
type filterKey struct {
	pid     int
	channel string
}

// changeFilter mengingat nilai terakhir yang tersimpan per produk dan channel.
type changeFilter struct {
	mu   sync.Mutex
	last map[filterKey]float64
}

func newChangeFilter() *changeFilter {
	return &changeFilter{last: map[filterKey]float64{}}
}

// Changed true bila nilai belum pernah tersimpan atau sudah berbeda cukup jauh
// dari nilai terakhir yang tersimpan. Tidak mengubah state; panggil Commit setelah
// INSERT berhasil.
func (f *changeFilter) Changed(pid int, channel string, v float64) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	last, ok := f.last[filterKey{pid, channel}]
	if !ok {
		return true
	}
	tol := changeTolerance[channel]
	if tol <= 0 {
		return v != last
	}
	// 1e-6: selisih 0.1 sering terhitung 0.0999999 akibat floating point
	// (mis. 20.3 - 20.2), jangan sampai perubahan sebesar tolerance dibuang.
	return math.Abs(v-last) >= tol-1e-6
}

// Commit mencatat nilai yang baru saja berhasil disimpan.
func (f *changeFilter) Commit(pid int, channel string, v float64) {
	f.mu.Lock()
	f.last[filterKey{pid, channel}] = v
	f.mu.Unlock()
}
