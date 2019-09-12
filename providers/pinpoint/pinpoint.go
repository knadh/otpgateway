package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/pinpoint"
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
	AppID       string `json:"AppID"`
	Region      string `json:"Region"`
	MessageType string `json:"MessageType"`
	SenderID    string `json:"SenderID"`
}

// New returns an instance of the SMS package. cfg is configuration
// represented as a JSON string. Supported options are.
// {
// 	AppID: "", // Application ID for amazon pinpoint service,
// 	Region: "", // AWS region name,
// 	MessageType: "", // MessageType to signify if it is transactional sms,
// 	SenderID: "" // Unique sender id
// }
func New(jsonCfg []byte) (interface{}, error) {
	var c *cfg
	if err := json.Unmarshal(jsonCfg, &c); err != nil {
		return nil, err
	}
	if c.AppID == "" {
		return nil, errors.New("invalid AppID")
	}
	if c.Region == "" {
		return nil, errors.New("invalid Region")
	}

	sess := session.Must(session.NewSession())
	svc := pinpoint.New(sess, aws.NewConfig().WithRegion(c.Region))

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
func (s *sms) Push(to, subject string, body []byte) error {
	var msg = string(body)

	payload := &pinpoint.SendMessagesInput{
		ApplicationId: &s.cfg.AppID,
		MessageRequest: &pinpoint.MessageRequest{
			Addresses: map[string]*pinpoint.AddressConfiguration{
				to: &pinpoint.AddressConfiguration{
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
