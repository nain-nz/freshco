package mqttsub

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"smartfood/internal/model"
)

func (s *Subscriber) publish(topic string, retained bool, payload any) error {
	if !s.client.IsConnectionOpen() {
		return errors.New("belum terhubung ke broker")
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	t := s.client.Publish(topic, 1, retained, b)
	if !t.WaitTimeout(5 * time.Second) {
		return errors.New("timeout mengirim ke broker")
	}
	return t.Error()
}

// PublishSettings mengirim pengaturan pendingin ke box dengan serial tertentu.
// Retained, supaya box yang baru menyala langsung mendapat nilai terakhir.
// Keputusan nyala/mati dihitung sendiri oleh box dari suhu, sedangkan kecepatan fan
// diatur user (fan_speed). Aturan keamanan di firmware: Peltier tidak boleh menyala
// kalau fan dinonaktifkan (sisi panas Peltier butuh didinginkan).
func (s *Subscriber) PublishSettings(serial string, st model.Settings) error {
	return s.publish(fmt.Sprintf("%s/%s/cooling/set", topicRoot, serial), true, map[string]any{
		"target_temp":     st.TargetTemp,
		"peltier_enabled": st.PeltierEnabled,
		"fan_enabled":     st.FanEnabled,
		"fan_speed":       st.FanSpeed,
	})
}

// SyncSettings membaca pengaturan pendingin tiap produk dari database lalu
// mengirimnya ke box masing-masing. Dipanggil setiap backend (re)connect ke broker.
func (s *Subscriber) SyncSettings() {
	rows, err := s.db.Query("SELECT id, serial FROM products WHERE claimed_at IS NOT NULL")
	if err != nil {
		log.Printf("[mqtt] sync: gagal membaca produk: %v", err)
		return
	}
	type prod struct {
		id     int
		serial string
	}
	var list []prod
	for rows.Next() {
		var p prod
		if err := rows.Scan(&p.id, &p.serial); err == nil {
			list = append(list, p)
		}
	}
	rows.Close()

	for _, p := range list {
		target, ok := s.setting(p.id, "target_temp")
		if !ok {
			continue
		}
		speed, ok := s.setting(p.id, "fan_speed")
		if !ok {
			speed = 70
		}
		st := model.Settings{
			TargetTemp:     target,
			PeltierEnabled: s.flag(p.id, "peltier_enabled"),
			FanEnabled:     s.flag(p.id, "fan_enabled"),
			FanSpeed:       speed,
		}
		if err := s.PublishSettings(p.serial, st); err != nil {
			log.Printf("[mqtt] sync %s gagal: %v", p.serial, err)
		}
	}
}

// flag membaca setting true/false milik produk; kalau belum ada dianggap true.
func (s *Subscriber) flag(pid int, key string) bool {
	var v string
	if err := s.db.QueryRow(
		"SELECT `value` FROM settings WHERE product_id = ? AND `key` = ?", pid, key).Scan(&v); err != nil {
		return true
	}
	return v != "false"
}
