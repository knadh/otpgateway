package main

import (
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis"
	"github.com/go-chi/chi/v5"
	"github.com/knadh/otpgateway/v3/internal/store/redis"
	"github.com/knadh/otpgateway/v3/pkg/models"
	"github.com/stretchr/testify/assert"
)

type dummyProv struct{}

// ID returns the Provider's ID.
func (d *dummyProv) ID() string {
	return dummyProvider
}

// ChannelName returns the e-mail Provider's name.
func (d *dummyProv) ChannelName() string {
	return "dummychannel"
}

// AddressName returns the label of the address.
func (d *dummyProv) AddressName() string {
	return "dummyaddress"
}

// ChannelDesc returns help text for the e-mail verification Provider.
func (d *dummyProv) ChannelDesc() string {
	return "dummy channel description"
}

// AddressDesc returns help text for the address.
func (d *dummyProv) AddressDesc() string {
	return "dummy address description"
}

// ValidateAddress "validates" an e-mail address.
func (d *dummyProv) ValidateAddress(to string) error {
	if to != dummyToAddress {
		return errors.New("invalid dummy to address")
	}
	return nil
}

// Push pushes an e-mail to the SMTP server.
func (d *dummyProv) Push(to models.OTP, subject string, m []byte) error {
	return nil
}

// MaxOTPLen returns the maximum allowed length of the OTP value.
func (d *dummyProv) MaxOTPLen() int {
	return 6
}

// MaxAddressLen returns the maximum allowed length of the 'to' address.
func (d *dummyProv) MaxAddressLen() int {
	return 6
}

// MaxBodyLen returns the max permitted body size.
func (d *dummyProv) MaxBodyLen() int {
	return 100 * 1024
}

const (
	dummyNamespace = "myapp"
	dummySecret    = "mysecret"
	dummyProvider  = "dummyprovider"
	dummyOTPID     = "myotp123"
	dummyToAddress = "dummy@to.com"
	dummyOTP       = "123456"
)

var (
	srv  *httptest.Server
	rdis *miniredis.Miniredis
)

func init() {
	// Dummy Redis.
	rd, err := miniredis.Run()
	if err != nil {
		log.Println(err)
	}
	rdis = rd
	port, _ := strconv.Atoi(rd.Port())

	// Provider templates.
	tpl := template.New("dummy")
	tpl, _ = tpl.Parse("test {{ .OTP }}")

	// Dummy app.
	app := &App{
		lo:        initLogger(true),
		providers: map[string]*provider{dummyProvider: &provider{provider: &dummyProv{}}},
		providerTpls: map[string]*providerTpl{
			dummyProvider: &providerTpl{
				subject: tpl,
				body:    tpl,
			},
		},
		constants: constants{
			OtpTTL:         10 * time.Second,
			OtpMaxAttempts: 10,
		},
		store: redis.New(redis.Conf{
			Host: rd.Host(),
			Port: port,
		}),
	}

	authCreds := map[string]string{dummyNamespace: dummySecret}
	r := chi.NewRouter()
	r.Get("/api/providers", auth(authCreds, wrap(app, handleGetProviders)))
	r.Get("/api/health", auth(authCreds, wrap(app, handleHealthCheck)))
	r.Put("/api/otp/{id}", auth(authCreds, wrap(app, handleSetOTP)))
	r.Post("/api/otp/{id}", auth(authCreds, wrap(app, handleVerifyOTP)))
	r.Delete("/api/otp/{id}/status", auth(authCreds, wrap(app, handleCheckOTPStatus)))
	r.Get("/otp/{namespace}/{id}", wrap(app, handleOTPView))
	r.Post("/otp/{namespace}/{id}", wrap(app, handleOTPView))
	srv = httptest.NewServer(r)
}

func reset() {
	rdis.FlushDB()
}

func TestGetProviders(t *testing.T) {
	var out httpResp
	r := testRequest(t, http.MethodGet, "/api/providers", nil, &out)
	assert.Equal(t, http.StatusOK, r.StatusCode, "non 200 response")
	assert.Equal(t, out.Data, []interface{}{dummyProvider}, "providers don't match")
}

func TestHealthCheck(t *testing.T) {
	var out httpResp
	r := testRequest(t, http.MethodGet, "/api/health", nil, &out)
	assert.Equal(t, http.StatusOK, r.StatusCode, "non 200 response")
}

func TestSetOTP(t *testing.T) {
	rdis.FlushDB()
	var (
		data = &otpResp{}
		out  = httpResp{
			Data: data,
		}
		p = url.Values{}
	)
	p.Set("to", dummyToAddress)
	p.Set("provider", "badprovider")

	// Register an OTP with a bad provider.
	r := testRequest(t, http.MethodPut, "/api/otp/"+dummyOTPID, p, &out)
	assert.Equal(t, http.StatusBadRequest, r.StatusCode, "non 400 response for bad provider")

	// Register an OTP with a bad to address.
	p.Set("to", "xxxx")
	r = testRequest(t, http.MethodPut, "/api/otp/"+dummyOTPID, p, &out)
	assert.Equal(t, http.StatusBadRequest, r.StatusCode, "non 400 response for bad to address")

	// Register without ID and OTP.
	p.Set("provider", dummyProvider)
	p.Set("to", dummyToAddress)
	r = testRequest(t, http.MethodPut, "/api/otp/"+dummyOTPID, p, &out)
	assert.Equal(t, http.StatusOK, r.StatusCode, "non 200 response")
	assert.Equal(t, dummyToAddress, data.OTP.To, "to doesn't match")
	assert.Equal(t, 1, data.OTP.Attempts, "attempts doesn't match")
	assert.NotEqual(t, "", data.OTP.ID, "id wasn't auto generated")
	assert.NotEqual(t, "", data.OTP.ID, "otp wasn't auto generated")

	// Register with known data.
	p.Set("id", dummyOTPID)
	p.Set("otp", dummyOTP)
	r = testRequest(t, http.MethodPut, "/api/otp/"+dummyOTPID, p, &out)
	assert.Equal(t, dummyOTPID, data.OTP.ID, "id doesn't match")
	assert.Equal(t, dummyOTP, data.OTP.OTP, "otp doesn't match")
}

func TestCheckOTP(t *testing.T) {
	rdis.FlushDB()
	var (
		data = &otpResp{}
		out  = httpResp{
			Data: data,
		}
		p = url.Values{}
	)
	p.Set("id", dummyOTPID)
	p.Set("otp", dummyOTP)
	p.Set("to", dummyToAddress)
	p.Set("provider", dummyProvider)

	// Register OTP.
	r := testRequest(t, http.MethodPut, "/api/otp/"+dummyOTPID, p, &out)
	assert.Equal(t, http.StatusOK, r.StatusCode, "otp registration failed")

	// Check OTP.
	cp := url.Values{}
	r = testRequest(t, http.MethodPost, "/api/otp/"+dummyOTPID, cp, &out)
	assert.Equal(t, http.StatusBadRequest, r.StatusCode, "non 400 response for empty otp check")

	// Bad OTP.
	cp.Set("otp", "123")
	r = testRequest(t, http.MethodPost, "/api/otp/"+dummyOTPID, cp, &out)
	assert.Equal(t, http.StatusBadRequest, r.StatusCode, "non 400 response for bad otp check")
	assert.Equal(t, 2, data.Attempts, "attempts didn't increase")

	// Good OTP. skip_delete so that it's not deleted.
	cp.Set("otp", dummyOTP)
	cp.Set("skip_delete", "true")
	r = testRequest(t, http.MethodPost, "/api/otp/"+dummyOTPID, cp, &data)
	assert.Equal(t, http.StatusOK, r.StatusCode, "good OTP failed")

	// Check it again. Shouldn't been deleted.
	cp.Set("skip_delete", "false")
	r = testRequest(t, http.MethodPost, "/api/otp/"+dummyOTPID, cp, &data)
	assert.Equal(t, http.StatusOK, r.StatusCode, "good OTP failed")

	// Check it again. Should be deleted.
	r = testRequest(t, http.MethodPost, "/api/otp/"+dummyOTPID, cp, &data)
	assert.NotEqual(t, http.StatusOK, r.StatusCode, "OTP didn't get deleted on verification")
}

func TestCheckOTPAttempts(t *testing.T) {
	rdis.FlushDB()
	var (
		data = &otpResp{}
		out  = httpResp{
			Data: data,
		}
		p = url.Values{}
	)
	p.Set("id", dummyOTPID)
	p.Set("otp", dummyOTP)
	p.Set("max_attempts", "5")
	p.Set("to", dummyToAddress)
	p.Set("provider", dummyProvider)

	// Register OTP.
	r := testRequest(t, http.MethodPut, "/api/otp/"+dummyOTPID, p, &out)
	assert.Equal(t, http.StatusOK, r.StatusCode, "otp registration failed")

	cp := url.Values{}
	cp.Set("otp", dummyOTP)
	cp.Set("skip_delete", "true")

	// Good otp check.
	r = testRequest(t, http.MethodPost, "/api/otp/"+dummyOTPID, cp, &data)
	assert.Equal(t, http.StatusOK, r.StatusCode, "good OTP failed")

	// Exceed bad attempts.
	cp.Set("otp", "123999")
	for i := 0; i < 10; i++ {
		r = testRequest(t, http.MethodPost, "/api/otp/"+dummyOTPID, cp, &data)
		assert.NotEqual(t, http.StatusOK, r.StatusCode, "bad OTP passed")
	}

	// Rate limited.
	cp.Set("skip_delete", "true")
	assert.Equal(t, http.StatusTooManyRequests, r.StatusCode, "bad OTPs didn't get rate limited")
}

func TestDeleteOnOTPCheck(t *testing.T) {
	rdis.FlushDB()
	var (
		data = &otpResp{}
		out  = httpResp{
			Data: data,
		}
		p = url.Values{}
	)
	p.Set("id", dummyOTPID)
	p.Set("otp", dummyOTP)
	p.Set("max_attempts", "5")
	p.Set("to", dummyToAddress)
	p.Set("provider", dummyProvider)

	// Register OTP.
	r := testRequest(t, http.MethodPut, "/api/otp/"+dummyOTPID, p, &out)
	assert.Equal(t, http.StatusOK, r.StatusCode, "otp registration failed")

	// Verification pending before otp status check.
	r = testRequest(t, http.MethodDelete, "/api/otp/"+dummyOTPID+"/status", nil, &data)
	assert.Equal(t, http.StatusBadRequest, r.StatusCode, "verification pending")

	cp := url.Values{}
	cp.Set("otp", dummyOTP)
	cp.Set("skip_delete", "true")

	// Verify (done by the otpgateway module).
	r = testRequest(t, http.MethodPost, "/api/otp/"+dummyOTPID, cp, &data)
	assert.Equal(t, http.StatusOK, r.StatusCode, "verification failed")

	// Delete on status check
	r = testRequest(t, http.MethodDelete, "/api/otp/"+dummyOTPID+"/status", nil, &data)
	assert.Equal(t, http.StatusOK, r.StatusCode, "verification pending")

	// Reattempt status check
	r = testRequest(t, http.MethodDelete, "/api/otp/"+dummyOTPID+"/status", nil, &data)
	assert.Equal(t, http.StatusBadRequest, r.StatusCode, "otp not found")
}

func testRequest(t *testing.T, method, path string, p url.Values, out interface{}) *http.Response {
	req, err := http.NewRequest(method, srv.URL+path, strings.NewReader(p.Encode()))
	if err != nil {
		t.Fatal(err)
		return nil
	}
	req.SetBasicAuth(dummyNamespace, dummySecret)
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	// HTTP client.
	c := &http.Client{}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
		return nil
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
		return nil
	}
	defer resp.Body.Close()

	if err := json.Unmarshal(respBody, out); err != nil {
		t.Fatal(err)
	}

	return resp
}
