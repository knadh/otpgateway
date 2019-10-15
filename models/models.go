package models

import (
	"encoding/json"
	"time"
)

// OTP contains the information about an OTP.
type OTP struct {
	Namespace   string          `redis:"namespace" json:"namespace"`
	ID          string          `redis:"id" json:"id"`
	To          string          `redis:"to" json:"to"`
	ChannelDesc string          `redis:"channel_description" json:"channel_description"`
	AddressDesc string          `redis:"address_description" json:"address_description"`
	Extra       json.RawMessage `redis:"extra" json:"extra"`
	Provider    string          `redis:"provider" json:"provider"`
	OTP         string          `redis:"otp" json:"otp"`
	MaxAttempts int             `redis:"max_attempts" json:"max_attempts"`
	Attempts    int             `redis:"attempts" json:"attempts"`
	Closed      bool            `redis:"closed" json:"closed"`
	TTL         time.Duration   `redis:"-" json:"-"`
	TTLSeconds  float64         `redis:"-" json:"ttl"`
}
