package service

import (
	"reflect"
	"testing"
)

func TestResolveTableChannels(t *testing.T) {
	all, err := resolveTableChannels(nil)
	if err != nil || !reflect.DeepEqual(all, sensorChannels) {
		t.Fatalf("default harus semua sensor, dapat %v (err %v)", all, err)
	}

	// urutan mengikuti sensorChannels dan duplikat dibuang
	got, err := resolveTableChannels([]string{"gas", "suhu_atas", "gas"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"suhu_atas", "gas"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("want %v, got %v", want, got)
	}

	// peltier dan fan bukan sensor, nama asal-asalan juga ditolak
	for _, bad := range []string{"peltier", "fan", "xx"} {
		if _, err := resolveTableChannels([]string{bad}); err == nil {
			t.Fatalf("%q seharusnya ditolak", bad)
		}
	}
}

func TestInClause(t *testing.T) {
	if got := inClause(3); got != "(?,?,?)" {
		t.Fatalf("got %s", got)
	}
}
