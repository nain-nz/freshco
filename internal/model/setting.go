package model

const (
	KeyTargetTemp        = "target_temp"
	KeyGasThreshold      = "gas_threshold"
	KeyWeightToleranceG  = "weight_tolerance_g"
	KeyTempToleranceC    = "temp_tolerance_c"
	KeyAlertTempEnabled  = "alert_temp_enabled"
	KeyAlertGasEnabled   = "alert_gas_enabled"
	KeyAlertWeightDownOn = "alert_weight_down_enabled"
	KeyAlertWeightUpOn   = "alert_weight_up_enabled"
	KeyPeltierEnabled    = "peltier_enabled"
	KeyFanEnabled        = "fan_enabled"
	KeyFanSpeed          = "fan_speed"
)

type Settings struct {
	TargetTemp   float64 `json:"target_temp"`
	GasThreshold float64 `json:"gas_threshold"`

	// Toleransi berat (g): alert kalau berat berubah lebih dari ini dalam 1 menit
	// terakhir (dibandingkan dengan data sensor sebelumnya, tanpa acuan tersimpan).
	WeightToleranceG float64 `json:"weight_tolerance_g"`
	// Toleransi suhu (C): alert kalau suhu rata-rata di atas target + toleransi
	// selama beberapa menit.
	TempToleranceC float64 `json:"temp_tolerance_c"`

	// Saklar tiap jenis alert.
	AlertTempEnabled       bool `json:"alert_temp_enabled"`
	AlertGasEnabled        bool `json:"alert_gas_enabled"`
	AlertWeightDownEnabled bool `json:"alert_weight_down_enabled"` // berat berkurang
	AlertWeightUpEnabled   bool `json:"alert_weight_up_enabled"`   // berat bertambah

	PeltierEnabled bool    `json:"peltier_enabled"` // false = Peltier dinonaktifkan
	FanEnabled     bool    `json:"fan_enabled"`     // false = fan dinonaktifkan
	FanSpeed       float64 `json:"fan_speed"`       // kecepatan fan (%), 10-100, diatur user
}

// DefaultSettings = isi tabel settings untuk setiap produk baru.
var DefaultSettings = [][2]string{
	{KeyTargetTemp, "8"},
	{KeyGasThreshold, "400"},
	{KeyWeightToleranceG, "50"},
	{KeyTempToleranceC, "3"},
	{KeyAlertTempEnabled, "true"},
	{KeyAlertGasEnabled, "true"},
	{KeyAlertWeightDownOn, "true"},
	{KeyAlertWeightUpOn, "true"},
	{KeyPeltierEnabled, "true"},
	{KeyFanEnabled, "true"},
	{KeyFanSpeed, "70"},
}
