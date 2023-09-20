// webhook is a generic webhook Provider implementation that posts OTP
// requests to a URL. This provider can be reused any number of times
// by defining multiple webhook providers in the app config.
package webhook

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/knadh/otpgateway/v3/pkg/models"
)

// Webhook is the default representation of the Webhook interface.
type Webhook struct {
	cfg        Config
	authHeader string
	http       *http.Client
}

// Webhook payload that is posted to the upstream URL.
type Payload struct {
	OTP     models.OTP `json:"otp"`
	Subject string     `json:"subject"`
	Body    string     `json:"body"`
}

// Config contains the webhook provider configuration.
type Config struct {
	URL           string `json:"url"`
	ID            string `json:"id"`
	Username      string `json:"username"`
	Password      string `json:"password"`
	Subject       string `json:"subject"`
	ChannelName   string `json:"channel_name"`
	AddressName   string `json:"address_name"`
	MaxAddressLen int    `json:"max_address_len"`
	MaxOTPLen     int    `json:"max_otp_len"`

	Timeout  time.Duration `json:"timeout"`
	MaxConns int           `json:"max_conns"`
}

// New implements a Kaleyra SMS provider.
func New(cfg Config) (*Webhook, error) {
	// Initialize the HTTP client.
	if cfg.Timeout.Seconds() < 1 {
		cfg.Timeout = time.Second * 3
	}
	if cfg.MaxConns < 1 {
		cfg.MaxConns = 1
	}

	authHeader := ""
	if cfg.Username != "" && cfg.Password != "" {
		authHeader = fmt.Sprintf("Basic %s", base64.StdEncoding.EncodeToString(
			[]byte(cfg.Username+":"+cfg.Password)))
	}

	return &Webhook{
		cfg:        cfg,
		authHeader: authHeader,
		http: &http.Client{
			Timeout: cfg.Timeout,
			Transport: &http.Transport{
				MaxIdleConnsPerHost:   cfg.MaxConns,
				ResponseHeaderTimeout: cfg.Timeout,
			},
		},
	}, nil
}

// ID returns the Provider's ID.
func (w *Webhook) ID() string {
	return w.cfg.ID
}

// ChannelName returns the Provider's name.
func (w *Webhook) ChannelName() string {
	return w.cfg.ChannelName
}

// AddressName returns the e-mail Provider's address name.
func (w *Webhook) AddressName() string {
	return w.cfg.AddressName
}

// ChannelDesc returns help text for the SMS verification Provider.
func (w *Webhook) ChannelDesc() string {
	return fmt.Sprintf(`A %d digit code has been sent to your %s.
		Enter it here to verify your %s.`, w.cfg.MaxOTPLen, w.cfg.ChannelName, w.cfg.AddressName)
}

// AddressDesc returns help text for the phone number.
func (w *Webhook) AddressDesc() string {
	return fmt.Sprintf("Please enter your %s", w.cfg.AddressName)
}

// ValidateAddress "validates" a phone number.
func (w *Webhook) ValidateAddress(to string) error {
	return nil
}

// Push pushes out an SMS.
func (w *Webhook) Push(otp models.OTP, subject string, body []byte) error {
	p := Payload{
		Subject: subject,
		Body:    string(body),
		OTP:     otp,
	}

	b, err := json.Marshal(p)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, w.cfg.URL, bytes.NewReader(b))
	if err != nil {
		return err
	}

	req.Header.Set("User-Agent", "otpgateway")
	req.Header.Add("Content-Type", "application/json")

	// Optional BasicAuth.
	if w.authHeader != "" {
		req.Header.Set("Authorization", w.authHeader)
	}

	resp, err := w.http.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		// Drain and close the body to let the Transport reuse the connection
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	return nil
}

// MaxAddressLen returns the maximum allowed length for the mobile number.
func (w *Webhook) MaxAddressLen() int {
	return w.cfg.MaxAddressLen
}

// MaxOTPLen returns the maximum allowed length of the OTP value.
func (w *Webhook) MaxOTPLen() int {
	return w.cfg.MaxOTPLen
}

// MaxBodyLen returns the max permitted body size.
func (w *Webhook) MaxBodyLen() int {
	return 0
}
