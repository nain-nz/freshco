package service

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"smartfood/internal/model"
)

// stripPhone membuang spasi dan tanda baca umum dari nomor HP.
func stripPhone(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case ' ', '-', '.', '(', ')':
			return -1
		}
		return r
	}, strings.TrimSpace(s))
}

// normalizePhone: boleh diawali +, sisanya angka, 9-15 digit.
func normalizePhone(s string) (string, error) {
	p := stripPhone(s)
	digits := strings.TrimPrefix(p, "+")
	if len(digits) < 9 || len(digits) > 15 {
		return "", NewValidationError("nomor HP harus 9-15 digit")
	}
	for _, r := range digits {
		if r < '0' || r > '9' {
			return "", NewValidationError("nomor HP hanya boleh berisi angka")
		}
	}
	return p, nil
}

func normalizeBirthDate(s string) (string, error) {
	t, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(s), time.Local)
	if err != nil {
		return "", NewValidationError("format tanggal lahir tidak valid")
	}
	if t.After(time.Now()) || t.Year() < 1900 {
		return "", NewValidationError("tanggal lahir tidak masuk akal")
	}
	return t.Format("2006-01-02"), nil
}

func normalizeAddress(s string) (string, error) {
	a := strings.TrimSpace(s)
	if n := utf8.RuneCountInString(a); n < 5 || n > 255 {
		return "", NewValidationError("alamat harus 5-255 karakter")
	}
	return a, nil
}

// ValidateProfile memeriksa dan menormalkan data diri.
func ValidateProfile(phone, birthDate, address string) (model.Profile, error) {
	var p model.Profile
	var err error
	if p.Phone, err = normalizePhone(phone); err != nil {
		return p, err
	}
	if p.BirthDate, err = normalizeBirthDate(birthDate); err != nil {
		return p, err
	}
	if p.Address, err = normalizeAddress(address); err != nil {
		return p, err
	}
	return p, nil
}

// BirthDateCooldown = jarak minimal antar perubahan tanggal lahir.
const BirthDateCooldown = 7 * 24 * time.Hour

var bulanID = [...]string{"Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agu", "Sep", "Okt", "Nov", "Des"}

// fmtTanggalID: "20 Nov 2026".
func fmtTanggalID(t time.Time) string {
	t = t.In(time.Local)
	return fmt.Sprintf("%d %s %d", t.Day(), bulanID[t.Month()-1], t.Year())
}

// checkBirthDateChange menentukan apakah [newBirth] termasuk PERUBAHAN tanggal lahir
// yang harus dicatat. Mengisi pertama kali (akun lama yang masih kosong) atau mengirim
// nilai yang sama bukan perubahan. Perubahan ditolak selama masih dalam masa tunggu.
func checkBirthDateChange(cur *model.User, newBirth string, now time.Time) (bool, error) {
	if cur.BirthDate == nil || *cur.BirthDate == newBirth {
		return false, nil
	}
	if cur.BirthDateLockedUntil != nil && cur.BirthDateLockedUntil.After(now) {
		return false, NewValidationError(
			"tanggal lahir baru bisa diubah lagi pada " + fmtTanggalID(*cur.BirthDateLockedUntil))
	}
	return true, nil
}
