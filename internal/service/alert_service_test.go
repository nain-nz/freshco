package service

import (
	"errors"
	"testing"
)

// DeleteMany menolak daftar kosong atau terlalu besar sebelum menyentuh database.
func TestDeleteManyValidasi(t *testing.T) {
	s := NewAlertService(nil)

	cases := map[string][]int{
		"kosong":         nil,
		"terlalu_banyak": make([]int, maxDeleteBatch+1),
	}
	for name, ids := range cases {
		_, err := s.DeleteMany(1, ids)
		var ve *ValidationError
		if !errors.As(err, &ve) {
			t.Errorf("%s: harapkan ValidationError, dapat %v", name, err)
		}
	}
}
