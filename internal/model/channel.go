package model

type Channel struct {
	ID       int    `json:"id"`
	DeviceID int    `json:"device_id"`
	Name     string `json:"name"`
}
