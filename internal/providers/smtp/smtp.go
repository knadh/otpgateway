package smtp

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net/smtp"
	"regexp"
	"time"

	"github.com/knadh/otpgateway/v3/pkg/models"
	"github.com/knadh/smtppool"
)

const (
	providerID    = "smtp"
	channelName   = "E-mail"
	addressName   = "E-mail ID"
	maxOTPlen     = 6
	maxAddressLen = 100
	maxBodyLen    = 100 * 1024
)

// http://www.golangprograms.com/regular-expression-to-validate-email-address.html
var reMail = regexp.MustCompile("^[a-zA-Z0-9.!#$%&'*+/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$")

// Config represents an SMTP server's credentials.
type Config struct {
	Host         string        `json:"host"`
	Port         int           `json:"port"`
	AuthProtocol string        `json:"auth_protocol"`
	Username     string        `json:"username"`
	Password     string        `json:"password"`
	FromEmail    string        `json:"from_email"`
	Timeout      time.Duration `json:"timeout"`
	MaxConns     int           `json:"max_conns"`

	// STARTTLS or TLS.
	TLSType       string `json:"tls_type"`
	TLSSkipVerify bool   `json:"tls_skip_verify"`
}

// SMTP is a generic SMTP e-mail provider.
type SMTP struct {
	cfg Config
	p   *smtppool.Pool
}

// New creates and returns an e-mail Provider backend.
func New(cfg Config) (*SMTP, error) {
	if cfg.FromEmail == "" {
		cfg.FromEmail = "otp@localhost"
	}

	// Initialize the SMTP mailer.
	var auth smtp.Auth
	switch cfg.AuthProtocol {
	case "login":
		auth = &smtppool.LoginAuth{Username: cfg.Username, Password: cfg.Password}
	case "cram":
		auth = smtp.CRAMMD5Auth(cfg.Username, cfg.Password)
	case "plain":
		auth = smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
	case "", "none":
	default:
		return nil, fmt.Errorf("unknown SMTP auth type '%s'", cfg.AuthProtocol)
	}

	opt := smtppool.Opt{
		Host:            cfg.Host,
		Port:            cfg.Port,
		MaxConns:        cfg.MaxConns,
		IdleTimeout:     time.Second * 10,
		PoolWaitTimeout: cfg.Timeout,
		Auth:            auth,
	}

	// TLS config.
	if cfg.TLSType != "none" {
		opt.TLSConfig = &tls.Config{}
		if cfg.TLSSkipVerify {
			opt.TLSConfig.InsecureSkipVerify = cfg.TLSSkipVerify
		} else {
			opt.TLSConfig.ServerName = cfg.Host
		}

		// SSL/TLS, not cfg.
		if cfg.TLSType == "TLS" {
			opt.SSL = true
		}
	}

	pool, err := smtppool.New(opt)
	if err != nil {
		return nil, err
	}

	return &SMTP{
		p:   pool,
		cfg: cfg,
	}, nil
}

// ID returns the Provider's ID.
func (s *SMTP) ID() string {
	return providerID
}

// ChannelName returns the e-mail Provider's name.
func (s *SMTP) ChannelName() string {
	return channelName
}

// ChannelDesc returns help text for the e-mail verification Provider.
func (s *SMTP) ChannelDesc() string {
	return fmt.Sprintf(`
	A %d digit code has been e-mailed to you.
	Please check your e-mail and enter the code here
	to complete the verification.`, maxOTPlen)
}

// AddressName returns the e-mail Provider's address name.
func (s *SMTP) AddressName() string {
	return addressName
}

// AddressDesc returns the help text that is shown to the end users when
// they're asked to enter their addresses (eg: e-mail or phone), if the OTP
// registered without an address.
func (s *SMTP) AddressDesc() string {
	return `Please enter the e-mail ID you want to verify`
}

// ValidateAddress "validates" an e-mail address.
func (s *SMTP) ValidateAddress(to string) error {
	if !reMail.MatchString(to) {
		return errors.New("invalid e-mail address")
	}
	return nil
}

// Push pushes an e-mail to the SMTP server.
func (s *SMTP) Push(otp models.OTP, subject string, m []byte) error {
	return s.p.Send(smtppool.Email{
		From:    s.cfg.FromEmail,
		To:      []string{otp.To},
		Subject: subject,
		HTML:    m,
	})
}

// MaxAddressLen returns the maximum allowed length of the e-mail address.
func (s *SMTP) MaxAddressLen() int {
	return maxAddressLen
}

// MaxOTPLen returns the maximum allowed length of the OTP value.
func (s *SMTP) MaxOTPLen() int {
	return maxOTPlen
}

// MaxBodyLen returns the max permitted body size.
func (s *SMTP) MaxBodyLen() int {
	return maxBodyLen
}
