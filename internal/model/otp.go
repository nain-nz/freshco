package model

import "time"

type OTP struct {
	ID        int
	CodeHash  string
	Attempts  int
	ExpiresAt time.Time
	UsedAt    *time.Time
}
