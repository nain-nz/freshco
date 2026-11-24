package service

import (
	"database/sql"
	"log"
	"strconv"

	"smartfood/internal/model"
)

// CoolingPublisher dipenuhi oleh mqttsub.Subscriber (kirim pengaturan ke Device 2).
type CoolingPublisher interface {
	PublishSettings(serial string, st model.Settings) error
}

type SettingService struct {
	db  *sql.DB
	pub CoolingPublisher
}

func NewSettingService(db *sql.DB, pub CoolingPublisher) *SettingService {
	return &SettingService{db: db, pub: pub}
}

type UpdateSettingsInput struct {
	TargetTemp       *float64 `json:"target_temp"`
	GasThreshold     *float64 `json:"gas_threshold"`
	WeightToleranceG *float64 `json:"weight_tolerance_g"`
	TempToleranceC   *float64 `json:"temp_tolerance_c"`

	AlertTempEnabled       *bool `json:"alert_temp_enabled"`
	AlertGasEnabled        *bool `json:"alert_gas_enabled"`
	AlertWeightDownEnabled *bool `json:"alert_weight_down_enabled"`
	AlertWeightUpEnabled   *bool `json:"alert_weight_up_enabled"`

	PeltierEnabled *bool    `json:"peltier_enabled"`
	FanEnabled     *bool    `json:"fan_enabled"`
	FanSpeed       *float64 `json:"fan_speed"`
}

type UpdateSettingsResult struct {
	Settings model.Settings `json:"settings"`
	// Synced false = tersimpan di database, tapi gagal dikirim ke box lewat MQTT.
	Synced bool `json:"synced"`
}

func (s *SettingService) Get(pid int) (model.Settings, error) {
	m, err := s.all(pid)
	if err != nil {
		return model.Settings{}, err
	}
	st := model.Settings{TargetTemp: 8, GasThreshold: 400, WeightToleranceG: 50, TempToleranceC: 3,
		AlertTempEnabled: true, AlertGasEnabled: true,
		AlertWeightDownEnabled: true, AlertWeightUpEnabled: true,
		PeltierEnabled: true, FanEnabled: true, FanSpeed: 70}
	num := func(key string, dst *float64) {
		if v, ok := m[key]; ok {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				*dst = f
			}
		}
	}
	num(model.KeyTargetTemp, &st.TargetTemp)
	num(model.KeyGasThreshold, &st.GasThreshold)
	num(model.KeyWeightToleranceG, &st.WeightToleranceG)
	num(model.KeyTempToleranceC, &st.TempToleranceC)
	num(model.KeyFanSpeed, &st.FanSpeed)
	if v, ok := m[model.KeyPeltierEnabled]; ok {
		st.PeltierEnabled = v != "false"
	}
	flag := func(key string, dst *bool) {
		if v, ok := m[key]; ok {
			*dst = v != "false"
		}
	}
	flag(model.KeyAlertTempEnabled, &st.AlertTempEnabled)
	flag(model.KeyAlertGasEnabled, &st.AlertGasEnabled)
	flag(model.KeyAlertWeightDownOn, &st.AlertWeightDownEnabled)
	flag(model.KeyAlertWeightUpOn, &st.AlertWeightUpEnabled)
	if v, ok := m[model.KeyFanEnabled]; ok {
		st.FanEnabled = v != "false"
	}
	return st, nil
}

func (s *SettingService) Update(pid int, in UpdateSettingsInput) (*UpdateSettingsResult, error) {
	changes := map[string]string{}
	fmtNum := func(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

	if in.TargetTemp != nil {
		if *in.TargetTemp < 0 || *in.TargetTemp > 30 {
			return nil, NewValidationError("target_temp harus antara 0 dan 30")
		}
		changes[model.KeyTargetTemp] = fmtNum(*in.TargetTemp)
	}
	if in.GasThreshold != nil {
		if *in.GasThreshold <= 0 {
			return nil, NewValidationError("gas_threshold harus lebih dari 0")
		}
		changes[model.KeyGasThreshold] = fmtNum(*in.GasThreshold)
	}
	if in.TempToleranceC != nil {
		if *in.TempToleranceC < 0 || *in.TempToleranceC > 30 {
			return nil, NewValidationError("temp_tolerance_c harus antara 0 dan 30")
		}
		changes[model.KeyTempToleranceC] = fmtNum(*in.TempToleranceC)
	}
	for _, f := range []struct {
		val *bool
		key string
	}{
		{in.AlertTempEnabled, model.KeyAlertTempEnabled},
		{in.AlertGasEnabled, model.KeyAlertGasEnabled},
		{in.AlertWeightDownEnabled, model.KeyAlertWeightDownOn},
		{in.AlertWeightUpEnabled, model.KeyAlertWeightUpOn},
	} {
		if f.val != nil {
			changes[f.key] = strconv.FormatBool(*f.val)
		}
	}
	if in.WeightToleranceG != nil {
		if *in.WeightToleranceG < 0 {
			return nil, NewValidationError("weight_tolerance_g tidak boleh negatif")
		}
		changes[model.KeyWeightToleranceG] = fmtNum(*in.WeightToleranceG)
	}
	if in.PeltierEnabled != nil {
		changes[model.KeyPeltierEnabled] = strconv.FormatBool(*in.PeltierEnabled)
	}
	if in.FanEnabled != nil {
		changes[model.KeyFanEnabled] = strconv.FormatBool(*in.FanEnabled)
	}
	if in.FanSpeed != nil {
		if *in.FanSpeed < 10 || *in.FanSpeed > 100 {
			return nil, NewValidationError("fan_speed harus antara 10 dan 100")
		}
		changes[model.KeyFanSpeed] = fmtNum(*in.FanSpeed)
	}
	if len(changes) == 0 {
		return nil, NewValidationError("tidak ada pengaturan yang diubah")
	}

	if err := s.setMany(pid, changes); err != nil {
		return nil, err
	}
	cur, err := s.Get(pid)
	if err != nil {
		return nil, err
	}

	synced := true
	// gas dan berat hanya dipakai backend; sisanya perlu diketahui box
	if in.TargetTemp != nil || in.PeltierEnabled != nil || in.FanEnabled != nil ||
		in.FanSpeed != nil {
		if err := s.publish(pid, cur); err != nil {
			log.Printf("[setting] gagal kirim ke box: %v", err)
			synced = false
		}
	}
	return &UpdateSettingsResult{Settings: cur, Synced: synced}, nil
}

// query database

// publish mengirim pengaturan ke box milik produk pid (topic memakai serial produk).
func (s *SettingService) publish(pid int, st model.Settings) error {
	var serial string
	if err := s.db.QueryRow("SELECT serial FROM products WHERE id = ?", pid).Scan(&serial); err != nil {
		return err
	}
	return s.pub.PublishSettings(serial, st)
}

func (s *SettingService) all(pid int) (map[string]string, error) {
	rows, err := s.db.Query("SELECT `key`, `value` FROM settings WHERE product_id = ?", pid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

func (s *SettingService) setMany(pid int, values map[string]string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for k, v := range values {
		if _, err := tx.Exec(
			"INSERT INTO settings (product_id, `key`, `value`) VALUES (?, ?, ?) "+
				"ON DUPLICATE KEY UPDATE `value` = VALUES(`value`)", pid, k, v); err != nil {
			return err
		}
	}
	return tx.Commit()
}
