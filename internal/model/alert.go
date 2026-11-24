package model

import "time"

type Alert struct {
	ID           int       `json:"id"`
	Time         time.Time `json:"time"`
	Channel      string    `json:"channel"`
	Type         string    `json:"type"`
	Value        *float64  `json:"value"`
	Acknowledged bool      `json:"acknowledged"`
}
