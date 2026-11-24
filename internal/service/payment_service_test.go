package service

import (
	"testing"
	"time"
)

func TestBuildSchedule(t *testing.T) {
	day := time.Date(2026, 11, 7, 0, 0, 0, 0, time.Local)
	cases := []struct {
		tenor int
		want  []int // jumlah per tagihan: DP lalu cicilan
	}{
		{1, []int{100000, 400000}},
		{2, []int{100000, 200000, 200000}},
		{4, []int{100000, 100000, 100000, 100000, 100000}},
		{6, []int{100000, 66666, 66666, 66666, 66666, 66666, 66670}}, // sisa pembulatan di cicilan terakhir
	}
	for _, c := range cases {
		got := BuildSchedule(500000, 100000, c.tenor, 0, day)
		if len(got) != len(c.want) {
			t.Fatalf("tenor %d: %d tagihan, mau %d", c.tenor, len(got), len(c.want))
		}
		sum := 0
		for i, in := range got {
			if in.Seq != i || in.Amount != c.want[i] {
				t.Errorf("tenor %d tagihan %d: seq=%d amount=%d, mau amount=%d", c.tenor, i, in.Seq, in.Amount, c.want[i])
			}
			sum += in.Amount
		}
		if sum != 500000 {
			t.Errorf("tenor %d: total %d, mau 500000", c.tenor, sum)
		}
	}

	// jatuh tempo: langsung = hari ini; cicilan = k bulan setelah hari ini
	s2 := BuildSchedule(500000, 100000, 2, 0, day)
	if !s2[0].Due.Equal(day) || !s2[1].Due.Equal(day.AddDate(0, 1, 0)) || !s2[2].Due.Equal(day.AddDate(0, 2, 0)) {
		t.Errorf("jatuh tempo tenor 2 salah: %v", s2)
	}
	s1 := BuildSchedule(500000, 100000, 1, 0, day)
	if !s1[1].Due.Equal(day) {
		t.Errorf("bayar langsung harus jatuh tempo hari ini, dapat %v", s1[1].Due)
	}
}

func TestBuildScheduleSubscriptionFee(t *testing.T) {
	day := time.Date(2026, 11, 7, 0, 0, 0, 0, time.Local)

	// dicicil: fee digabung ke tiap cicilan (satu kali bayar sebulan), DP tanpa fee
	got := BuildSchedule(500000, 100000, 4, 15000, day)
	if len(got) != 5 {
		t.Fatalf("%d tagihan, mau 5", len(got))
	}
	if got[0].Amount != 100000 || got[0].Fee != 0 {
		t.Errorf("DP salah: %+v", got[0])
	}
	product := got[0].Amount
	for _, in := range got[1:] {
		if in.Amount != 115000 || in.Fee != 15000 {
			t.Errorf("cicilan %d: amount=%d fee=%d, mau 115000/15000", in.Seq, in.Amount, in.Fee)
		}
		product += in.Amount - in.Fee
	}
	if product != 500000 {
		t.Errorf("bagian produk %d, mau 500000", product)
	}

	// bayar langsung jatuh tempo hari ini: tanpa fee (langganan mulai bulan depan)
	one := BuildSchedule(500000, 100000, 1, 15000, day)
	if one[1].Amount != 400000 || one[1].Fee != 0 {
		t.Errorf("pelunasan langsung salah: %+v", one[1])
	}
}

func TestCodes(t *testing.T) {
	c, err := randomCode(claimCodeLen)
	if err != nil || len(c) != claimCodeLen {
		t.Fatalf("randomCode: %q %v", c, err)
	}
	f := formatCode(c)
	if len(f) != claimCodeLen+2 || f[4] != '-' || f[9] != '-' {
		t.Errorf("formatCode salah: %q", f)
	}
	// kode yang diketik user (huruf kecil, spasi, tanda -) harus sama hash-nya
	if hashCode(normalizeCode(" "+toLower(f)+" ")) != hashCode(c) {
		t.Error("normalizeCode tidak menyamakan format kode")
	}
}

func toLower(s string) string {
	b := []byte(s)
	for i, ch := range b {
		if ch >= 'A' && ch <= 'Z' {
			b[i] = ch + 32
		}
	}
	return string(b)
}

func TestAttemptLimiter(t *testing.T) {
	l := newAttemptLimiter(3, time.Minute)
	for i := 0; i < 3; i++ {
		if !l.Allow(1) {
			t.Fatalf("percobaan %d harus diizinkan", i+1)
		}
	}
	if l.Allow(1) {
		t.Error("percobaan ke-4 harus ditolak")
	}
	if !l.Allow(2) {
		t.Error("user lain tidak boleh ikut terblokir")
	}
}
