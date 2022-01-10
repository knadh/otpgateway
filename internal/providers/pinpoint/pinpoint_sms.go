package pinpoint_sms

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/pinpoint"
	"github.com/knadh/otpgateway/v3/internal/models"
)

const (
	providerID    = "pinpoint"
	channelName   = "SMS"
	addressName   = "Mobile number"
	maxAddresslen = 10
	maxOTPlen     = 6
)

var (
	channelType = "SMS"
)

var reNum = regexp.MustCompile(`\+?([0-9]){8,15}`)

// Pinpoint implements the AWS Pinpoint SMS provider.
type Pinpoint struct {
	cfg Config
	p   *pinpoint.Pinpoint
}

type Config struct {
	ApplicationID       string `json:"application_id"`
	AccessKey           string `json:"access_key"`
	SecretKey           string `json:"secret_key"`
	Region              string `json:"region"`
	SMSSenderID         string `json:"sms_sender_id"`
	SMSMessageType      string `json:"sms_message_type"`
	SMSTemplateID       string `json:"sms_template_id"`
	DefaultPhoneCode string `json:"default_phone_code"`

	MaxConns int           `json:"max_conns"`
	Timeout  time.Duration `json:"timeout"`
}

// NewSMS returns an instance of the SMS package. cfg is configuration
// represented as a JSON string. Supported options are.
// {
// 	AppID: "", // Application ID for amazon pinpoint service,
// 	AWSAccessKey: "", // AWS access key,
// 	AWSSecretKey: "", // AWS secret key,
// 	AWSRegion: "", // AWS region name,
// 	MessageType: "", // MessageType to signify if it is transactional sms,
// 	SenderID: "" // Unique sender id
// }
func NewSMS(cfg Config) (interface{}, error) {
	if cfg.ApplicationID == "" {
		return nil, errors.New("invalid application_id")
	}
	if cfg.Region == "" {
		return nil, errors.New("invalid region")
	}
	if cfg.AccessKey == "" {
		return nil, errors.New("invalid access_key")
	}
	if cfg.SecretKey == "" {
		return nil, errors.New("invalid secret_key")
	}

	if cfg.MaxConns < 1 {
		cfg.MaxConns = 1
	}
	if cfg.Timeout.Seconds() < 1 {
		cfg.Timeout = time.Second * 3
	}

	p := pinpoint.New(session.Must(session.NewSession()),
		aws.NewConfig().
			WithCredentials(credentials.NewStaticCredentials(cfg.AccessKey, cfg.SecretKey, "")).
			WithRegion(cfg.Region).
			WithHTTPClient(&http.Client{
				Timeout: cfg.Timeout,
				Transport: &http.Transport{
					MaxIdleConnsPerHost:   cfg.MaxConns,
					IdleConnTimeout:       90 * time.Second,
					ResponseHeaderTimeout: cfg.Timeout,
				},
			}),
	)

	return &Pinpoint{cfg: cfg, p: p}, nil
}

// ID returns the Provider's ID.
func (p *Pinpoint) ID() string {
	return providerID
}

// ChannelName returns the Provider's name.
func (p *Pinpoint) ChannelName() string {
	return channelName
}

// AddressName returns the e-mail Provider's address name.
func (p *Pinpoint) AddressName() string {
	return addressName
}

// ChannelDesc returns help text for the SMS verification Provider.
func (p *Pinpoint) ChannelDesc() string {
	return fmt.Sprintf(`
		A %d digit code has been sent as an SMS to your mobile.
		Enter it here to verify your mobile number.`, maxOTPlen)
}

// AddressDesc returns help text for the phone number.
func (p *Pinpoint) AddressDesc() string {
	return "Please enter your mobile number"
}

// ValidateAddress "validates" a phone number.
func (p *Pinpoint) ValidateAddress(to string) error {
	if !reNum.MatchString(to) {
		return errors.New("invalid mobile number")
	}
	return nil
}

// Push pushes out an SMS.
func (p *Pinpoint) Push(otp models.OTP, subject string, body []byte) error {
	msg := string(body)

	payload := &pinpoint.SendMessagesInput{
		ApplicationId: &p.cfg.ApplicationID,
		MessageRequest: &pinpoint.MessageRequest{
			Addresses: map[string]*pinpoint.AddressConfiguration{
				p.sanitizePhone(otp.To): &pinpoint.AddressConfiguration{
					ChannelType: &channelType,
				},
			},
			MessageConfiguration: &pinpoint.DirectMessageConfiguration{
				SMSMessage: &pinpoint.SMSMessage{
					Body:        &msg,
					MessageType: &p.cfg.SMSMessageType,
					SenderId:    &p.cfg.SMSSenderID,
					TemplateId:  &p.cfg.SMSTemplateID,
				},
			},
		},
	}
	if _, err := p.p.SendMessages(payload); err != nil {
		return err
	}

	return nil
}

// MaxAddressLen returns the maximum allowed length for the mobile number.
func (p *Pinpoint) MaxAddressLen() int {
	return maxAddresslen
}

// MaxOTPLen returns the maximum allowed length of the OTP value.
func (p *Pinpoint) MaxOTPLen() int {
	return maxOTPlen
}

// MaxBodyLen returns the max permitted body size.
func (p *Pinpoint) MaxBodyLen() int {
	return 140
}

func (p *Pinpoint) sanitizePhone(phone string) string {
	phone = strings.TrimSpace(phone)

	if strings.HasPrefix(phone, "+") {
		return phone
	} else if strings.HasPrefix(phone, "00") {
		return "+" + phone[2:]
	}

	return p.cfg.DefaultPhoneCode + phone
}
