package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"github.com/knadh/otpgateway/models"
)

const (
	providerID    = "solsms"
	channelName   = "SMS"
	addressName   = "Mobile number"
	maxAddresslen = 10
	maxOTPlen     = 6
	apiURL        = "https://api-alerts.kaleyra.com/v4/"
	statusOK      = "OK"
)

var reNum = regexp.MustCompile(`\+?([0-9]){8,15}`)

// sms is the default representation of the sms interface.
type sms struct {
	cfg *cfg
	h   *http.Client
}

type cfg struct {
	RootURL      string `json:"RootURL"`
	APIKey       string `json:"APIKey"`
	Sender       string `json:"Sender"`
	Timeout      int    `json:"Timeout"`
	MaxIdleConns int    `json:"MaxIdleConns"`
}

// solSMSAPIResp represents the response from solsms API.
type solSMSAPIResp struct {
	Status  string      `json:"status"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

// New returns an instance of the SMS package. cfg is configuration
// represented as a JSON string. Supported options are.
// {
// 	RootURL: "", // Optional root URL of the API,
// 	APIKey: "", // API Key,
// 	Sender: "", // Sender name
// 	Timeout: 5 // Optional HTTP timeout in seconds
// }
func New(jsonCfg []byte) (interface{}, error) {
	var c *cfg
	if err := json.Unmarshal(jsonCfg, &c); err != nil {
		return nil, err
	}
	if c.RootURL == "" {
		c.RootURL = apiURL
	}
	if c.APIKey == "" || c.Sender == "" {
		return nil, errors.New("invalid APIKey or Sender")
	}

	// Initialize the HTTP client.
	t := 5
	if c.Timeout != 0 {
		t = c.Timeout
	}
	h := &http.Client{
		Timeout: time.Duration(t) * time.Second,
		Transport: &http.Transport{
			MaxIdleConnsPerHost:   1,
			ResponseHeaderTimeout: time.Second * time.Duration(t),
		},
	}

	return &sms{
		cfg: c,
		h:   h}, nil
}

// ID returns the Provider's ID.
func (s *sms) ID() string {
	return providerID
}

// ChannelName returns the Provider's name.
func (s *sms) ChannelName() string {
	return channelName
}

// AddressName returns the e-mail Provider's address name.
func (*sms) AddressName() string {
	return addressName
}

// ChannelDesc returns help text for the SMS verification Provider.
func (s *sms) ChannelDesc() string {
	return fmt.Sprintf(`
		We've sent a %d digit code in an SMS to your mobile.
		Enter it here to verify your mobile number.`, maxOTPlen)
}

// AddressDesc returns help text for the phone number.
func (s *sms) AddressDesc() string {
	return "Please enter your mobile number"
}

// ValidateAddress "validates" a phone number.
func (s *sms) ValidateAddress(to string) error {
	if !reNum.MatchString(to) {
		return errors.New("invalid mobile number")
	}
	return nil
}

// Push pushes out an SMS.
func (s *sms) Push(otp models.OTP, subject string, body []byte) error {
	var p = url.Values{}
	p.Set("method", "sms")
	p.Set("api_key", s.cfg.APIKey)
	p.Set("sender", s.cfg.Sender)
	p.Set("to", otp.To)
	p.Set("message", string(body))

	// Make the request.
	resp, err := s.h.PostForm(s.cfg.RootURL, p)
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
	r := solSMSAPIResp{}
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}
	if r.Status != statusOK {
		return errors.New(r.Message)
	}
	return nil
}

// MaxAddressLen returns the maximum allowed length for the mobile number.
func (s *sms) MaxAddressLen() int {
	return maxAddresslen
}

// MaxOTPLen returns the maximum allowed length of the OTP value.
func (s *sms) MaxOTPLen() int {
	return maxOTPlen
}

// MaxBodyLen returns the max permitted body size.
func (s *sms) MaxBodyLen() int {
	return 140
}
