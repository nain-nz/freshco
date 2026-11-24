package mqttsub

import "testing"

func TestChangeFilter(t *testing.T) {
	f := newChangeFilter()

	// belum pernah tersimpan -> selalu berubah
	if !f.Changed(1, "suhu_atas", 20.0) {
		t.Fatal("nilai pertama harus dianggap berubah")
	}
	f.Commit(1, "suhu_atas", 20.0)

	cases := []struct {
		ch   string
		v    float64
		want bool
		why  string
	}{
		{"suhu_atas", 20.0, false, "nilai sama"},
		{"suhu_atas", 20.04, false, "selisih 0.04 < 0.1"},
		{"suhu_atas", 20.1, true, "selisih tepat 0.1 harus tersimpan"},
		{"suhu_atas", 19.9, true, "turun 0.1"},
		{"suhu_atas", 25, true, "lonjakan besar"},
	}
	for _, c := range cases {
		if got := f.Changed(1, c.ch, c.v); got != c.want {
			t.Errorf("%s: Changed(%s,%v)=%v, mau %v", c.why, c.ch, c.v, got, c.want)
		}
	}

	// bug float klasik: 20.3-20.2 = 0.0999999..., tetap harus dianggap berubah
	f.Commit(1, "suhu_kiri", 20.2)
	if !f.Changed(1, "suhu_kiri", 20.3) {
		t.Error("20.2 -> 20.3 harus dianggap berubah (masalah floating point)")
	}

	// drift pelan: tiap langkah 0.04, baru tersimpan setelah total >= 0.1
	f.Commit(1, "suhu_bawah", 10.0)
	v, saved := 10.0, 0
	for i := 0; i < 10; i++ {
		v += 0.04
		if f.Changed(1, "suhu_bawah", v) {
			f.Commit(1, "suhu_bawah", v)
			saved++
		}
	}
	if saved != 3 { // 10.12, 10.24 (0.12), 10.36 -> 3 kali dalam 10 langkah
		t.Errorf("drift: tersimpan %d kali, mau 3", saved)
	}

	// gas / berat / status
	f.Commit(1, "gas", 150)
	if f.Changed(1, "gas", 153) || !f.Changed(1, "gas", 155) {
		t.Error("toleransi gas 5 ppm salah")
	}
	f.Commit(1, "berat", 500)
	if f.Changed(1, "berat", 501) || !f.Changed(1, "berat", 502) {
		t.Error("toleransi berat 2 g salah")
	}
	f.Commit(1, "peltier", 0)
	if f.Changed(1, "peltier", 0) || !f.Changed(1, "peltier", 1) {
		t.Error("status peltier 0/1 salah")
	}

	// channel tak dikenal: perbedaan sekecil apa pun = berubah, sama = tidak
	f.Commit(1, "xyz", 1.5)
	if f.Changed(1, "xyz", 1.5) || !f.Changed(1, "xyz", 1.5001) {
		t.Error("channel tanpa toleransi salah")
	}
}

// Nilai terakhir diingat per produk: produk 2 tidak terpengaruh data produk 1.
func TestChangeFilterPerProduct(t *testing.T) {
	f := newChangeFilter()
	f.Commit(1, "suhu_atas", 20.0)
	if f.Changed(1, "suhu_atas", 20.0) {
		t.Fatal("produk 1: nilai sama harus dianggap tidak berubah")
	}
	if !f.Changed(2, "suhu_atas", 20.0) {
		t.Fatal("produk 2 belum punya data, harus dianggap berubah")
	}
}
