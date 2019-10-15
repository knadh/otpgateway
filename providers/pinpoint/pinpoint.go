package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/pinpoint"
	"github.com/knadh/otpgateway/models"
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

// sms is the default representation of the sms interface.
type sms struct {
	cfg *cfg
	p   *pinpoint.Pinpoint
}

type cfg struct {
	AppID        string `json:"AppID"`
	AWSAccessKey string `json:"AWSAccessKey"`
	AWSSecretKey string `json:"AWSSecretKey"`
	AWSRegion    string `json:"AWSRegion"`
	MessageType  string `json:"MessageType"`
	SenderID     string `json:"SenderID"`
}

// New returns an instance of the SMS package. cfg is configuration
// represented as a JSON string. Supported options are.
// {
// 	AppID: "", // Application ID for amazon pinpoint service,
// 	AWSAccessKey: "", // AWS access key,
// 	AWSSecretKey: "", // AWS secret key,
// 	AWSRegion: "", // AWS region name,
// 	MessageType: "", // MessageType to signify if it is transactional sms,
// 	SenderID: "" // Unique sender id
// }
func New(jsonCfg []byte) (interface{}, error) {
	var c *cfg
	if err := json.Unmarshal(jsonCfg, &c); err != nil {
		return nil, err
	}

	// Validations.
	if c.AppID == "" {
		return nil, errors.New("invalid AppID")
	}
	if c.AWSRegion == "" {
		return nil, errors.New("invalid AWSRegion")
	}
	if c.AWSAccessKey == "" {
		return nil, errors.New("invalid AWSAccessKey")
	}
	if c.AWSSecretKey == "" {
		return nil, errors.New("invalid AWSSecretKey")
	}

	sess := session.Must(session.NewSession())
	svc := pinpoint.New(sess,
		aws.NewConfig().
			WithCredentials(credentials.NewStaticCredentials(c.AWSAccessKey, c.AWSSecretKey, "")).
			WithRegion(c.AWSRegion),
	)

	return &sms{
		cfg: c,
		p:   svc}, nil
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
	var msg = string(body)

	payload := &pinpoint.SendMessagesInput{
		ApplicationId: &s.cfg.AppID,
		MessageRequest: &pinpoint.MessageRequest{
			Addresses: map[string]*pinpoint.AddressConfiguration{
				sanitizePhone(otp.To): &pinpoint.AddressConfiguration{
					ChannelType: &channelType,
				},
			},
			MessageConfiguration: &pinpoint.DirectMessageConfiguration{
				SMSMessage: &pinpoint.SMSMessage{
					Body:        &msg,
					MessageType: &s.cfg.MessageType,
					SenderId:    &s.cfg.SenderID,
				},
			},
		},
	}
	if _, err := s.p.SendMessages(payload); err != nil {
		return err
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

func sanitizePhone(phone string) string {
	phone = strings.TrimSpace(phone)
	// If length is 10 then assume it as Indian phone number
	if len(phone) == 10 {
		return "+91" + phone
	} else if len(phone) > 10 && strings.HasPrefix(phone, "00") {
		return "+" + phone[2:]
	} else {
		return phone
	}
}
