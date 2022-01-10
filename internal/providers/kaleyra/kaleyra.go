package kaleyra

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"github.com/knadh/otpgateway/v3/internal/models"
)

const (
	providerID    = "kaleyra"
	channelName   = "SMS"
	addressName   = "Mobile number"
	maxAddresslen = 10
	maxOTPlen     = 6
	apiURL        = "https://api-alerts.kaleyra.com/v4/"
	statusOK      = "OK"
)

var reNum = regexp.MustCompile(`\+?([0-9]){8,15}`)

// Kaleyra is the default representation of the Kaleyra interface.
type Kaleyra struct {
	cfg Config
	h   *http.Client
}

type Config struct {
	APIKey   string        `json:"api_key"`
	Sender   string        `json:"sender"`
	Timeout  time.Duration `json:"timeout"`
	MaxConns int           `json:"max_conns"`
}

// apiResp represents the response from kaleyra API.
type apiResp struct {
	Status  string      `json:"status"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

// New implements a Kaleyra SMS provider.
func New(cfg Config) (*Kaleyra, error) {
	if cfg.APIKey == "" || cfg.Sender == "" {
		return nil, errors.New("invalid APIKey or Sender")
	}

	// Initialize the HTTP client.
	if cfg.Timeout.Seconds() < 1 {
		cfg.Timeout = time.Second * 3
	}

	return &Kaleyra{
		cfg: cfg,
		h: &http.Client{
			Timeout: cfg.Timeout,
			Transport: &http.Transport{
				MaxIdleConnsPerHost:   cfg.MaxConns,
				ResponseHeaderTimeout: cfg.Timeout,
			},
		},
	}, nil
}

// ID returns the Provider's ID.
func (k *Kaleyra) ID() string {
	return providerID
}

// ChannelName returns the Provider's name.
func (k *Kaleyra) ChannelName() string {
	return channelName
}

// AddressName returns the e-mail Provider's address name.
func (k *Kaleyra) AddressName() string {
	return addressName
}

// ChannelDesc returns help text for the SMS verification Provider.
func (k *Kaleyra) ChannelDesc() string {
	return fmt.Sprintf(`
		We've sent a %d digit code in an SMS to your mobile.
		Enter it here to verify your mobile number.`, maxOTPlen)
}

// AddressDesc returns help text for the phone number.
func (k *Kaleyra) AddressDesc() string {
	return "Please enter your mobile number"
}

// ValidateAddress "validates" a phone number.
func (k *Kaleyra) ValidateAddress(to string) error {
	if !reNum.MatchString(to) {
		return errors.New("invalid mobile number")
	}
	return nil
}

// Push pushes out an SMS.
func (k *Kaleyra) Push(otp models.OTP, subject string, body []byte) error {
	var p = url.Values{}
	p.Set("method", "sms")
	p.Set("api_key", k.cfg.APIKey)
	p.Set("sender", k.cfg.Sender)
	p.Set("to", otp.To)
	p.Set("message", string(body))

	// Make the request.
	resp, err := k.h.PostForm(apiURL, p)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Read the response.
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// We now unmarshal the body.
	r := apiResp{}
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}
	if r.Status != statusOK {
		return errors.New(r.Message)
	}
	return nil
}

// MaxAddressLen returns the maximum allowed length for the mobile number.
func (k *Kaleyra) MaxAddressLen() int {
	return maxAddresslen
}

// MaxOTPLen returns the maximum allowed length of the OTP value.
func (k *Kaleyra) MaxOTPLen() int {
	return maxOTPlen
}

// MaxBodyLen returns the max permitted body size.
func (k *Kaleyra) MaxBodyLen() int {
	return 140
}
