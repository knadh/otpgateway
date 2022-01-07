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

// ProviderConfig represents the common configuration types for a Provider.
type ProviderConfig struct {
	Template string `mapstructure:"template"`
	Subject  string `mapstructure:"subject"`
	Config   string `mapstructure:"config"`
}

// NewProvider represents an initialisation function that takes
// an arbitrary JSON encoded configuration set and returns an
// instance of a Provider.
type NewProvider func(jsonCfg []byte) (Provider, error)

// Provider is an interface for a generic messaging backend,
// for instance, e-mail, SMS etc.
type Provider interface {
	// ID returns the name of the Provider.
	ID() string

	// ChannelName returns the name of the channel the provider is
	// validating, for example "SMS" or "E-mail". This is displayed on
	// web views.
	ChannelName() string

	// ChannelDesc returns the help text that is shown to the end users describing
	// how the Provider handles OTP verification.
	// Eg: "We've sent a 6 digit code to your phone. Enter that here to verify
	//      your phone number"
	ChannelDesc() string

	// AddressName returns the name or label of the address for this provider.
	// For example "E-mail" for an e-mail provider or "Phone number" for an SMS provider.
	AddressName() string

	// AddressDesc returns the help text that is shown to the end users when
	// they're asked to enter their addresses (eg: e-mail or phone), if the OTP
	// registered without an address.
	AddressDesc() string

	// ValidateAddress validates the 'to' address the Provider
	// is supposed to send the OTP to, for instance, an e-mail
	// or a phone number.
	ValidateAddress(to string) error

	// Push pushes a message. Depending on the the Provider,
	// implementation, this can either cause the message to
	// be sent immediately or be queued waiting for a Flush().
	Push(otp OTP, subject string, body []byte) error

	// MaxAddressLen returns the maximum allowed length of the 'to' address.
	MaxAddressLen() int

	// MaxOTPLen returns the maximum allowed length of the OTP value.
	MaxOTPLen() int

	// MaxBodyLen returns the maximum permitted length of the text
	// that can be sent by the Provider.
	MaxBodyLen() int
}
