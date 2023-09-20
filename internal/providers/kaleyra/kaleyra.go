package kaleyra

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/knadh/otpgateway/v3/pkg/models"
)

const (
	ChannelSMS      = "SMS"
	ChannelWhatsapp = "WhatsApp"

	providerID    = "kaleyra"
	addressName   = "Mobile number"
	maxAddresslen = 10
	maxOTPlen     = 6
	apiURL        = "https://api.kaleyra.io/v1/%s/messages"
)

var reNum = regexp.MustCompile(`\+?([0-9]){8,15}`)

// Kaleyra is the default representation of the Kaleyra interface.
type Kaleyra struct {
	channel string
	apiURL  string
	cfg     Config
	h       *http.Client
}

type Config struct {
	APIKey           string        `json:"api_key"`
	SID              string        `json:"sid"`
	Sender           string        `json:"sender"`
	TemplateName     string        `json:"template_name"`
	DefaultPhoneCode string        `json:"default_phone_code"`
	Timeout          time.Duration `json:"timeout"`
	MaxConns         int           `json:"max_conns"`
}

// New implements a Kaleyra provider.
// type = SMS | WhatsApp
func New(channel string, cfg Config) (*Kaleyra, error) {
	if cfg.APIKey == "" || cfg.Sender == "" {
		return nil, errors.New("invalid APIKey or Sender")
	}

	// Initialize the HTTP client.
	if cfg.Timeout.Seconds() < 1 {
		cfg.Timeout = time.Second * 3
	}

	return &Kaleyra{
		channel: channel,
		apiURL:  fmt.Sprintf(apiURL, cfg.SID),
		cfg:     cfg,
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
	return k.channel
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
	p := url.Values{}
	p.Set("to", k.sanitizePhone(otp.To))

	if k.channel == ChannelSMS {
		p.Set("type", "OTP")
		p.Set("sender", k.cfg.Sender)
		p.Set("body", string(body))
	} else {
		p.Set("type", "template")
		p.Set("channel", "whatsapp")
		p.Set("from", k.cfg.Sender)
		p.Set("template_name", k.cfg.TemplateName)
		p.Set("params", fmt.Sprintf(`"%s"`, otp.OTP))
	}

	// Make the request.
	req, err := http.NewRequest(http.MethodPost, k.apiURL, bytes.NewReader([]byte(p.Encode())))
	if err != nil {
		return err
	}

	req.Header.Add("api-key", k.cfg.APIKey)
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	resp, err := k.h.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Read the response.
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode == http.StatusAccepted || resp.StatusCode == http.StatusOK {
		return nil
	}

	return errors.New(string(b))
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

func (k *Kaleyra) sanitizePhone(phone string) string {
	phone = strings.TrimSpace(phone)

	if strings.HasPrefix(phone, "+") {
		return phone
	} else if strings.HasPrefix(phone, "00") {
		return "+" + phone[2:]
	}

	return k.cfg.DefaultPhoneCode + phone
}
