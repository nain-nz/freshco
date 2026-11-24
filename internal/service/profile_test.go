package service

import (
	"strings"
	"testing"
	"time"

	"smartfood/internal/model"
)

func strp(s string) *string { return &s }

func TestCheckBirthDateChange(t *testing.T) {
	now := time.Date(2026, 11, 15, 10, 0, 0, 0, time.Local)
	future := now.Add(5 * 24 * time.Hour)
	past := now.Add(-time.Hour)

	cases := []struct {
		name        string
		cur         model.User
		newBirth    string
		wantChanged bool
		wantErr     bool
	}{
		{"isi pertama kali (akun lama)", model.User{}, "2001-03-12", false, false},
		{"nilai sama tidak dihitung", model.User{BirthDate: strp("2001-03-12"), BirthDateLockedUntil: &future}, "2001-03-12", false, false},
		{"ubah pertama setelah daftar", model.User{BirthDate: strp("2001-03-12")}, "2001-03-13", true, false},
		{"ubah saat masih terkunci", model.User{BirthDate: strp("2001-03-12"), BirthDateLockedUntil: &future}, "2001-03-13", false, true},
		{"masa tunggu sudah lewat", model.User{BirthDate: strp("2001-03-12"), BirthDateLockedUntil: &past}, "2001-03-13", true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			changed, err := checkBirthDateChange(&c.cur, c.newBirth, now)
			if (err != nil) != c.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, c.wantErr)
			}
			if changed != c.wantChanged {
				t.Fatalf("changed = %v, want %v", changed, c.wantChanged)
			}
			if err != nil && !strings.Contains(err.Error(), "20 Nov 2026") {
				t.Fatalf("pesan error harus memuat tanggal buka kunci, dapat %q", err.Error())
			}
		})
	}
}
