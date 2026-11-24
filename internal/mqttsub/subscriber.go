package mqttsub

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"smartfood/internal/config"
)

// Topic per produk: smartfood/{serial}/<kelompok>/<nama>. Backend subscribe dengan
// wildcard "+" pada posisi serial, lalu mengambil serial dari topic.
const (
	topicRoot = "smartfood"

	kindSuhu    = "sensor/suhu"
	kindGas     = "sensor/gas"
	kindBerat   = "sensor/berat"
	kindStatus  = "cooling/status"
	kindOnline1 = "device1/online"
	kindOnline2 = "device2/online"

	// jarak minimum antar alert dengan tipe + channel yang sama
	alertCooldown = 10 * time.Minute

	// berat: dibandingkan dengan data sensor dalam jendela waktu ini
	weightWindow = time.Minute
	// suhu: alert kalau terus di atas target + toleransi selama ini
	tempAlertAfter = 5 * time.Minute
)

type Subscriber struct {
	db      *sql.DB
	client  mqtt.Client
	changes *changeFilter // nilai terakhir yang tersimpan per (produk, channel), untuk melewati data kembar

	chMu     sync.RWMutex
	channels map[string]int // nama channel -> id (katalog global)

	prMu     sync.RWMutex
	products map[string]int // serial -> id produk

	// status untuk alert, dipisah per produk (hanya di memori; callback MQTT berjalan berurutan)
	states map[int]*productState
}

type productState struct {
	weightWin     []weightPoint
	tempHighSince time.Time
}

type weightPoint struct {
	at time.Time
	v  float64
}

type reading struct {
	channel string
	value   float64
}

func New(cfg config.Config, db *sql.DB) (*Subscriber, error) {
	s := &Subscriber{
		db: db, changes: newChangeFilter(),
		products: map[string]int{}, states: map[int]*productState{},
	}
	if err := s.loadChannels(); err != nil {
		return nil, err
	}
	s.seedLastSaved()

	opts := mqtt.NewClientOptions().
		AddBroker("tcp://" + net.JoinHostPort(cfg.MQTTHost, cfg.MQTTPort)).
		SetClientID("smartfood-backend").
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(3 * time.Second).
		SetKeepAlive(30 * time.Second)
	if cfg.MQTTUser != "" {
		opts.SetUsername(cfg.MQTTUser).SetPassword(cfg.MQTTPassword)
	}
	// subscribe ulang setiap kali (re)connect
	opts.SetOnConnectHandler(func(c mqtt.Client) {
		log.Println("[mqtt] terhubung ke broker")
		filters := map[string]byte{}
		for _, k := range []string{kindSuhu, kindGas, kindBerat, kindStatus, kindOnline1, kindOnline2} {
			filters[topicRoot+"/+/"+k] = 1
		}
		if t := c.SubscribeMultiple(filters, s.route); t.Wait() && t.Error() != nil {
			log.Printf("[mqtt] subscribe gagal: %v", t.Error())
		}
		go s.SyncSettings() // kirim pengaturan pendingin terakhir ke semua box
	})
	opts.SetConnectionLostHandler(func(_ mqtt.Client, err error) {
		log.Printf("[mqtt] koneksi terputus: %v", err)
	})

	s.client = mqtt.NewClient(opts)
	return s, nil
}

func (s *Subscriber) Start() {
	log.Println("[mqtt] menghubungkan ke broker...")
	s.client.Connect() // dengan ConnectRetry, akan terus mencoba di latar belakang
}

func (s *Subscriber) Stop() {
	s.client.Disconnect(250)
}

func (s *Subscriber) loadChannels() error {
	rows, err := s.db.Query("SELECT id, name FROM channels")
	if err != nil {
		return err
	}
	defer rows.Close()

	m := map[string]int{}
	for rows.Next() {
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return err
		}
		m[name] = id
	}
	if len(m) == 0 {
		return errors.New("tabel channels kosong, jalankan dulu: go run . seed")
	}
	s.chMu.Lock()
	s.channels = m
	s.chMu.Unlock()
	return rows.Err()
}

func (s *Subscriber) channelID(name string) (int, bool) {
	s.chMu.RLock()
	defer s.chMu.RUnlock()
	id, ok := s.channels[name]
	return id, ok
}

// seedLastSaved memuat nilai terakhir tiap (produk, channel) dari database, supaya
// setelah backend restart nilai yang sama dengan data tersimpan tidak ditulis ulang.
func (s *Subscriber) seedLastSaved() {
	rows, err := s.db.Query(
		"SELECT r.product_id, c.name, r.`value` FROM readings r JOIN channels c ON c.id = r.channel_id " +
			"WHERE r.`time` = (SELECT MAX(x.`time`) FROM readings x " +
			"WHERE x.product_id = r.product_id AND x.channel_id = r.channel_id)")
	if err != nil {
		log.Printf("[mqtt] gagal membaca nilai terakhir: %v", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var pid int
		var name string
		var v float64
		if err := rows.Scan(&pid, &name, &v); err == nil {
			s.changes.Commit(pid, name, v)
		}
	}
}

// productID mencari id produk dari serial (cache, lalu database). Serial yang
// tidak terdaftar ditolak: box asing tidak boleh menulis ke database.
func (s *Subscriber) productID(serial string) (int, bool) {
	s.prMu.RLock()
	id, ok := s.products[serial]
	s.prMu.RUnlock()
	if ok {
		return id, true
	}
	if err := s.db.QueryRow("SELECT id FROM products WHERE serial = ?", serial).Scan(&id); err != nil {
		return 0, false
	}
	s.prMu.Lock()
	s.products[serial] = id
	s.prMu.Unlock()
	return id, true
}

func (s *Subscriber) state(pid int) *productState {
	st, ok := s.states[pid]
	if !ok {
		st = &productState{}
		s.states[pid] = st
	}
	return st
}

// route memecah topic "smartfood/{serial}/{kelompok}/{nama}" lalu meneruskan ke handler.
func (s *Subscriber) route(_ mqtt.Client, msg mqtt.Message) {
	parts := strings.SplitN(msg.Topic(), "/", 3)
	if len(parts) != 3 || parts[0] != topicRoot {
		return
	}
	serial, kind := parts[1], parts[2]
	pid, ok := s.productID(serial)
	if !ok {
		log.Printf("[mqtt] serial tidak dikenal, diabaikan: %s", serial)
		return
	}

	var err error
	switch kind {
	case kindSuhu:
		err = s.handleSuhu(pid, msg.Payload())
	case kindGas:
		err = s.handleGas(pid, msg.Payload())
	case kindBerat:
		err = s.handleBerat(pid, msg.Payload())
	case kindStatus:
		err = s.handleStatus(pid, msg.Payload())
	case kindOnline1, kindOnline2:
		log.Printf("[online] %s %s = %s", serial, kind, msg.Payload())
	}
	if err != nil {
		log.Printf("[error] %s: %v", msg.Topic(), err)
	}
}

// handler per topic

func (s *Subscriber) handleSuhu(pid int, payload []byte) error {
	var p struct {
		Atas  *float64 `json:"atas"`  // posisi 2
		Kiri  *float64 `json:"kiri"`  // posisi 4
		Kanan *float64 `json:"kanan"` // posisi 6
		Bawah *float64 `json:"bawah"` // posisi 8
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return err
	}
	var rs []reading
	add := func(name string, v *float64) {
		// DS18B20 yang lepas/rusak biasanya mengirim -127, jangan disimpan
		if v != nil && *v > -100 {
			rs = append(rs, reading{name, *v})
		}
	}
	add("suhu_atas", p.Atas)
	add("suhu_kiri", p.Kiri)
	add("suhu_kanan", p.Kanan)
	add("suhu_bawah", p.Bawah)
	now := time.Now()
	if err := s.saveReadings(pid, now, rs); err != nil {
		return err
	}
	if len(rs) > 0 {
		var sum float64
		for _, r := range rs {
			sum += r.value
		}
		s.checkTemp(pid, now, sum/float64(len(rs)))
	}
	return nil
}

// checkTemp: alert kalau suhu rata-rata terus di atas target + toleransi selama
// tempAlertAfter (supaya pintu yang sebentar dibuka tidak memicu alert).
// Alert ini milik box secara keseluruhan, jadi tidak terikat satu channel.
func (s *Subscriber) checkTemp(pid int, now time.Time, avg float64) {
	st := s.state(pid)
	if !s.flag(pid, "alert_temp_enabled") {
		st.tempHighSince = time.Time{}
		return
	}
	target, ok := s.setting(pid, "target_temp")
	if !ok {
		return
	}
	tol, ok := s.setting(pid, "temp_tolerance_c")
	if !ok {
		tol = 3
	}
	if avg <= target+tol {
		st.tempHighSince = time.Time{}
		return
	}
	if st.tempHighSince.IsZero() {
		st.tempHighSince = now
		return
	}
	if now.Sub(st.tempHighSince) >= tempAlertAfter {
		s.raiseAlert(pid, now, "", "suhu_tinggi", avg)
	}
}

func (s *Subscriber) handleGas(pid int, payload []byte) error {
	var p struct {
		Value float64 `json:"value"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return err
	}
	now := time.Now()
	if err := s.saveReadings(pid, now, []reading{{"gas", p.Value}}); err != nil {
		return err
	}
	if th, ok := s.setting(pid, "gas_threshold"); ok && p.Value > th && s.flag(pid, "alert_gas_enabled") {
		s.raiseAlert(pid, now, "gas", "gas_tinggi", p.Value)
	}
	return nil
}

func (s *Subscriber) handleBerat(pid int, payload []byte) error {
	var p struct {
		Value float64 `json:"value"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return err
	}
	now := time.Now()
	if err := s.saveReadings(pid, now, []reading{{"berat", p.Value}}); err != nil {
		return err
	}
	tol, ok := s.setting(pid, "weight_tolerance_g")
	if !ok {
		tol = 50
	}
	s.checkWeight(pid, now, p.Value, tol)
	return nil
}

// checkWeight membandingkan berat sekarang dengan data sensor tertua dalam
// jendela weightWindow. Berubah lebih dari toleransi = alert (berkurang/bertambah),
// lalu jendela dimulai lagi supaya satu kejadian tidak dihitung berulang.
func (s *Subscriber) checkWeight(pid int, now time.Time, v, tol float64) {
	st := s.state(pid)
	keep := st.weightWin[:0]
	for _, w := range st.weightWin {
		if now.Sub(w.at) <= weightWindow {
			keep = append(keep, w)
		}
	}
	st.weightWin = append(keep, weightPoint{now, v})

	diff := v - st.weightWin[0].v
	switch {
	case diff < -tol:
		if s.flag(pid, "alert_weight_down_enabled") {
			s.raiseAlert(pid, now, "berat", "berat_berkurang", v)
		}
	case diff > tol:
		if s.flag(pid, "alert_weight_up_enabled") {
			s.raiseAlert(pid, now, "berat", "berat_bertambah", v)
		}
	default:
		return
	}
	st.weightWin = []weightPoint{{now, v}}
}

func (s *Subscriber) handleStatus(pid int, payload []byte) error {
	var p struct {
		Peltier float64 `json:"peltier"`
		Fan     float64 `json:"fan"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return err
	}
	return s.saveReadings(pid, time.Now(), []reading{
		{"peltier", p.Peltier},
		{"fan", p.Fan},
	})
}

// database

func (s *Subscriber) saveReadings(pid int, t time.Time, rs []reading) error {
	// Hanya simpan yang benar-benar berubah dari nilai tersimpan terakhir.
	changed := make([]reading, 0, len(rs))
	for _, r := range rs {
		if s.changes.Changed(pid, r.channel, r.value) {
			changed = append(changed, r)
		}
	}
	rs = changed
	if len(rs) == 0 {
		return nil // tidak ada perubahan: tidak menyentuh database
	}
	t = t.Truncate(time.Millisecond)

	holders := make([]string, 0, len(rs))
	args := make([]any, 0, len(rs)*4)
	parts := make([]string, 0, len(rs))
	for _, r := range rs {
		id, ok := s.channelID(r.channel)
		if !ok {
			return fmt.Errorf("channel %q tidak ada di database", r.channel)
		}
		holders = append(holders, "(?, ?, ?, ?)")
		args = append(args, pid, t, id, r.value)
		parts = append(parts, fmt.Sprintf("%s=%.2f", r.channel, r.value))
	}

	_, err := s.db.Exec(
		"INSERT INTO readings (product_id, `time`, channel_id, `value`) VALUES "+strings.Join(holders, ", ")+
			" ON DUPLICATE KEY UPDATE `value` = VALUES(`value`)", args...)
	if err != nil {
		return err
	}
	for _, r := range rs {
		s.changes.Commit(pid, r.channel, r.value)
	}
	log.Printf("[simpan] produk=%d %s", pid, strings.Join(parts, " "))
	return nil
}

func (s *Subscriber) setting(pid int, key string) (float64, bool) {
	var v string
	if err := s.db.QueryRow(
		"SELECT `value` FROM settings WHERE product_id = ? AND `key` = ?", pid, key).Scan(&v); err != nil {
		return 0, false
	}
	f, err := strconv.ParseFloat(v, 64)
	return f, err == nil
}

// raiseAlert menyimpan alert. channel kosong = alert milik box (tanpa channel).
func (s *Subscriber) raiseAlert(pid int, now time.Time, channel, alertType string, value float64) {
	var chID sql.NullInt64
	if channel != "" {
		id, ok := s.channelID(channel)
		if !ok {
			log.Printf("[alert] channel %q tidak ada", channel)
			return
		}
		chID = sql.NullInt64{Int64: int64(id), Valid: true}
	}

	var n int
	err := s.db.QueryRow(
		"SELECT COUNT(*) FROM alerts WHERE product_id = ? AND channel_id <=> ? AND type = ? AND `time` > ?",
		pid, chID, alertType, now.Add(-alertCooldown)).Scan(&n)
	if err != nil {
		log.Printf("[alert] gagal cek alert terakhir: %v", err)
		return
	}
	if n > 0 {
		return // masih dalam masa jeda, jangan spam
	}

	if _, err := s.db.Exec(
		"INSERT INTO alerts (product_id, `time`, channel_id, type, `value`) VALUES (?, ?, ?, ?, ?)",
		pid, now, chID, alertType, value); err != nil {
		log.Printf("[alert] gagal menyimpan: %v", err)
		return
	}
	log.Printf("[ALERT] produk=%d %s (nilai %.2f)", pid, alertType, value)
}
