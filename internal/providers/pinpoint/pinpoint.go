package pinpoint

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/pinpoint"
	"github.com/aws/aws-sdk-go-v2/service/pinpoint/types"
	"github.com/knadh/otpgateway/v3/pkg/models"
)

const (
	providerID    = "pinpoint"
	channelName   = "SMS"
	addressName   = "Mobile number"
	maxAddresslen = 10
	maxOTPlen     = 6
)

var reNum = regexp.MustCompile(`\+?([0-9]){8,15}`)

// PinpointSMS implements the AWS PinpointSMS SMS provider.
type PinpointSMS struct {
	cfg Config
	p   *pinpoint.Client
}

type Config struct {
	ApplicationID    string        `json:"application_id"`
	AccessKey        string        `json:"access_key"`
	SecretKey        string        `json:"secret_key"`
	Region           string        `json:"region"`
	SMSSenderID      string        `json:"sms_sender_id"`
	SMSMessageType   string        `json:"sms_message_type"`
	SMSEntityID      string        `json:"sms_entity_id"`
	SMSTemplateID    string        `json:"sms_template_id"`
	DefaultPhoneCode string        `json:"default_phone_code"`
	MaxConns         int           `json:"max_conns"`
	Timeout          time.Duration `json:"timeout"`
}

// NewSMS returns an instance of the SMS package. cfg is configuration
// represented as a JSON string. Supported options are.
//
//	{
//		AppID: "", // Application ID for amazon pinpoint service,
//		AWSAccessKey: "", // AWS access key,
//		AWSSecretKey: "", // AWS secret key,
//		AWSRegion: "", // AWS region name,
//		MessageType: "", // MessageType to signify if it is transactional or promotional sms,
//		SenderID: "" // Unique sender id
//	}

func NewSMS(cfg Config) (*PinpointSMS, error) {
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

	// Validate SMSMessageType
	if cfg.SMSMessageType != string(types.MessageTypeTransactional) && cfg.SMSMessageType != string(types.MessageTypePromotional) {
		return nil, errors.New("invalid SMSMessageType: must be TRANSACTIONAL or PROMOTIONAL")
	}

	cfgAws, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(cfg.Region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, "")),
	)
	if err != nil {
		return nil, err
	}

	return &PinpointSMS{cfg: cfg, p: pinpoint.NewFromConfig(cfgAws)}, nil
}

// ID returns the Provider's ID.
func (p *PinpointSMS) ID() string {
	return providerID
}

// ChannelName returns the Provider's name.
func (p *PinpointSMS) ChannelName() string {
	return channelName
}

// AddressName returns the e-mail Provider's address name.
func (p *PinpointSMS) AddressName() string {
	return addressName
}

// ChannelDesc returns help text for the SMS verification Provider.
func (p *PinpointSMS) ChannelDesc() string {
	return fmt.Sprintf(`
		A %d digit code has been sent as an SMS to your mobile.
		Enter it here to verify your mobile number.`, maxOTPlen)
}

// AddressDesc returns help text for the phone number.
func (p *PinpointSMS) AddressDesc() string {
	return "Please enter your mobile number"
}

// ValidateAddress "validates" a phone number.
func (p *PinpointSMS) ValidateAddress(to string) error {
	if !reNum.MatchString(to) {
		return errors.New("invalid mobile number")
	}
	return nil
}

func (p *PinpointSMS) Push(otp models.OTP, subject string, body []byte) error {
	input := &pinpoint.SendMessagesInput{
		ApplicationId: aws.String(p.cfg.ApplicationID),
		MessageRequest: &types.MessageRequest{
			Addresses: map[string]types.AddressConfiguration{
				p.sanitizePhone(otp.To): {
					ChannelType: types.ChannelTypeSms,
				},
			},
			MessageConfiguration: &types.DirectMessageConfiguration{
				SMSMessage: &types.SMSMessage{
					Body:        aws.String(string(body)),
					MessageType: types.MessageType(p.cfg.SMSMessageType),
					SenderId:    aws.String(p.cfg.SMSSenderID),
					EntityId:    aws.String(p.cfg.SMSEntityID),
					TemplateId:  aws.String(p.cfg.SMSTemplateID),
				},
			},
		},
	}

	_, err := p.p.SendMessages(context.TODO(), input)
	return err
}

// MaxAddressLen returns the maximum allowed length for the mobile number.
func (p *PinpointSMS) MaxAddressLen() int {
	return maxAddresslen
}

// MaxOTPLen returns the maximum allowed length of the OTP value.
func (p *PinpointSMS) MaxOTPLen() int {
	return maxOTPlen
}

// MaxBodyLen returns the max permitted body size.
func (p *PinpointSMS) MaxBodyLen() int {
	return 140
}

func (p *PinpointSMS) sanitizePhone(phone string) string {
	phone = strings.TrimSpace(phone)

	if strings.HasPrefix(phone, "+") {
		return phone
	} else if strings.HasPrefix(phone, "00") {
		return "+" + phone[2:]
	}

	return p.cfg.DefaultPhoneCode + phone
}
