package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi"
	"github.com/knadh/otpgateway"
	"github.com/knadh/otpgateway/models"
)

const (
	alphaChars    = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	numChars      = "0123456789"
	alphaNumChars = alphaChars + numChars

	actCheck  = "check"
	actResend = "resend"

	uriViewOTP     = "/otp/%s/%s"
	uriViewAddress = "/otp/%s/%s/address"
	uriCheck       = "/otp/%s/%s?otp=%s&action=check"
)

type httpResp struct {
	Status  string      `json:"status"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

type otpResp struct {
	models.OTP
	URL string `json:"url"`
}

type otpErrResp struct {
	TTL         float64 `json:"ttl_seconds"`
	Attempts    int     `json:"attempts"`
	MaxAttempts int     `json:"max_attempts"`
}

type tpl struct {
	Title       string
	Description string

	ChannelName   string
	ChannelDesc   string
	AddressName   string
	AddressDesc   string
	MaxAddressLen int
	MaxOTPLen     int
	OTP           models.OTP
	Locked        bool
	Closed        bool
	Message       string

	App *App
}

type pushTpl struct {
	To        string
	Namespace string
	Channel   string
	OTP       string
	OTPURL    string
}

// ErrNotExist is thrown when an OTP (requested by namespace / ID)
// does not exist.
var ErrNotExist = errors.New("the OTP does not exist")

// handleGetProviders returns the list of available message providers.
func handleGetProviders(w http.ResponseWriter, r *http.Request) {
	var (
		app = r.Context().Value("app").(*App)
		out = make([]string, len(app.providers))
	)
	i := 0
	for p := range app.providers {
		out[i] = p
		i++
	}
	sendResponse(w, out)
}

func handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	// check if store is reachable
	var (
		app = r.Context().Value("app").(*App)
		err = app.store.Ping()
	)
	if err != nil {
		sendErrorResponse(w, "unable to reach store", http.StatusServiceUnavailable, nil)
		return
	}
	sendResponse(w, "OK")
	return
}

// handleSetOTP creates a new OTP while respecting maximum attempts
// and TTL values.
func handleSetOTP(w http.ResponseWriter, r *http.Request) {
	var (
		app         = r.Context().Value("app").(*App)
		namespace   = r.Context().Value("namespace").(string)
		id          = chi.URLParam(r, "id")
		provider    = r.FormValue("provider")
		channelDesc = r.FormValue("channel_description")
		addressDesc = r.FormValue("address_description")
		extra       = []byte(r.FormValue("extra"))
		to          = r.FormValue("to")
		otpVal      = r.FormValue("otp")
	)

	// Get the provider.
	pro, ok := app.providers[provider]
	if !ok {
		sendErrorResponse(w, "unknown provider", http.StatusBadRequest, nil)
		return
	}

	// Validate the 'to' address with the provider if one is given.
	// If an address is not set, the gateway will render the address
	// collection UI.
	if to != "" {
		if err := pro.ValidateAddress(to); err != nil {
			sendErrorResponse(w, fmt.Sprintf("invalid `to` address: %v", err),
				http.StatusBadRequest, nil)
			return
		}
	}

	// If there's extra data, make sure it's JSON.
	if len(extra) > 0 {
		var tmp interface{}
		if err := json.Unmarshal(extra, &tmp); err != nil {
			sendErrorResponse(w, fmt.Sprintf("Invalid JSON in `extra`: %v", err), http.StatusBadRequest, nil)
			return
		}
	} else {
		extra = []byte("{}")
	}

	// If there is no incoming ID, generate a random ID.
	if len(id) < 6 {
		sendErrorResponse(w, "ID should be min 6 chars", http.StatusBadRequest, nil)
		return
	} else if id == "" {
		if i, err := generateRandomString(32, alphaNumChars); err != nil {
			app.logger.Printf("error generating ID: %v", err)
			sendErrorResponse(w, "error generating ID", http.StatusInternalServerError, nil)
			return
		} else {
			id = i
		}
	}

	// If there's no incoming OTP, generate a random one.
	if otpVal == "" {
		o, err := generateRandomString(pro.MaxOTPLen(), numChars)
		if err != nil {
			app.logger.Printf("error generating OTP: %v", err)
			sendErrorResponse(w, "error generating OTP", http.StatusInternalServerError, nil)
			return
		}
		otpVal = o
	}

	// Check if the OTP attempts have exceeded the quota.
	otp, err := app.store.Check(namespace, id, false)
	if err != nil && err != otpgateway.ErrNotExist {
		app.logger.Printf("error checking OTP status: %v", err)
		sendErrorResponse(w, "error checking OTP status", http.StatusBadRequest, nil)
		return
	}

	// There's an existing OTP that's locked.
	if err != otpgateway.ErrNotExist && isLocked(otp) {
		sendErrorResponse(w,
			fmt.Sprintf("OTP attempts exceeded. Retry after %0.f seconds.",
				otp.TTL.Seconds()),
			http.StatusTooManyRequests, otpErrResp{
				Attempts:    otp.Attempts,
				MaxAttempts: app.otpMaxAttempts,
				TTL:         otp.TTL.Seconds(),
			})
		return
	}

	// Create the OTP.
	newOTP, err := app.store.Set(namespace, id, models.OTP{
		OTP:         otpVal,
		To:          to,
		ChannelDesc: channelDesc,
		AddressDesc: addressDesc,
		Extra:       []byte(extra),
		Provider:    provider,
		TTL:         app.otpTTL,
		MaxAttempts: app.otpMaxAttempts,
	})
	if err != nil {
		app.logger.Printf("error setting OTP: %v", err)
		sendErrorResponse(w, "error setting OTP", http.StatusInternalServerError, nil)
		return
	}

	// Push the OTP out.
	if to != "" {
		if err := push(newOTP, app.providerTpls[pro.ID()], pro, app.RootURL); err != nil {
			app.logger.Printf("error sending OTP: %v", err)
			sendErrorResponse(w, "error sending OTP", http.StatusInternalServerError, nil)
			return
		}
	}

	out := otpResp{newOTP, getURL(app.RootURL, newOTP, false)}
	sendResponse(w, out)
}

// handleCheckOTPStatus checks the user input against a stored OTP.
func handleCheckOTPStatus(w http.ResponseWriter, r *http.Request) {
	var (
		app       = r.Context().Value("app").(*App)
		namespace = r.Context().Value("namespace").(string)
		id        = chi.URLParam(r, "id")
	)

	if len(id) < 6 {
		sendErrorResponse(w, "ID should be min 6 chars", http.StatusBadRequest, nil)
		return
	}

	// Check the OTP status.
	out, err := app.store.Check(namespace, id, false)
	if err != nil {
		if err == otpgateway.ErrNotExist {
			sendErrorResponse(w, err.Error(), http.StatusBadRequest, nil)
			return
		}

		app.logger.Printf("error checking OTP: %v", err)
		sendErrorResponse(w, err.Error(), http.StatusBadRequest, nil)
		return
	}

	if out.Closed {
		sendResponse(w, out)
		return
	}

	sendErrorResponse(w, "OTP not verified", http.StatusBadRequest, nil)
}

// handleVerifyOTP checks the user input against a stored OTP.
func handleVerifyOTP(w http.ResponseWriter, r *http.Request) {
	var (
		app       = r.Context().Value("app").(*App)
		namespace = r.Context().Value("namespace").(string)
		id        = chi.URLParam(r, "id")
		otpVal    = r.FormValue("otp")
	)

	if len(id) < 6 {
		sendErrorResponse(w, "ID should be min 6 chars", http.StatusBadRequest, nil)
		return
	}
	if otpVal == "" {
		sendErrorResponse(w, "`otp` is empty", http.StatusBadRequest, nil)
		return
	}

	out, err := verifyOTP(namespace, id, otpVal, app)
	if err != nil {
		code := http.StatusBadRequest
		if err == otpgateway.ErrNotExist {
			sendErrorResponse(w, err.Error(), code, nil)
			return
		}

		if out.Closed {
			code = http.StatusTooManyRequests
		}
		sendErrorResponse(w, err.Error(), code, out)
		return
	}

	sendResponse(w, out)
}

// handleOTPView renders the HTTP view.
func handleOTPView(w http.ResponseWriter, r *http.Request) {
	var (
		app       = r.Context().Value("app").(*App)
		namespace = chi.URLParam(r, "namespace")
		action    = r.FormValue("action")
		id        = chi.URLParam(r, "id")
		otp       = r.FormValue("otp")

		out    models.OTP
		otpErr error
	)

	if action == "" {
		// Render the view without incrementing attempts.
		out, otpErr = app.store.Check(namespace, id, false)
	} else if action == actResend {
		// Fetch the OTP for resending.
		out, otpErr = app.store.Check(namespace, id, true)
	} else {
		// Validate the attempt.
		out, otpErr = verifyOTP(namespace, id, otp, app)
	}
	if otpErr == otpgateway.ErrNotExist {
		app.tpl.ExecuteTemplate(w, "message", tpl{App: app,
			Title: "Session expired",
			Description: `Your session has expired.
					Please re-initiate the verification.`,
		})
		return
	}

	// Attempts are maxed out and locked.
	if isLocked(out) {
		app.tpl.ExecuteTemplate(w, "message", tpl{App: app,
			Title:       "Too many attempts",
			Description: fmt.Sprintf("Please retry after %d seconds.", int64(out.TTLSeconds)),
		})
		return
	}

	// Get the provider.
	pro, ok := app.providers[out.Provider]
	if !ok {
		app.tpl.ExecuteTemplate(w, "message", tpl{App: app,
			Title:       "Internal error",
			Description: "The provider for this OTP was not found.",
		})
		return
	}

	// OTP's already verified and closed.
	if out.Closed {
		app.tpl.ExecuteTemplate(w, "message", tpl{App: app,
			OTP:    out,
			Closed: true,
			Title:  fmt.Sprintf("%s verified", pro.ChannelName()),
			Description: fmt.Sprintf(
				`You %s is now verified. You can close this page now.`,
				pro.ChannelName()),
		})
		return
	}

	// There is no 'to' address set.
	if out.To == "" {
		http.Redirect(w, r, fmt.Sprintf(uriViewAddress, out.Namespace, out.ID),
			http.StatusFound)
		return
	}

	msg := ""
	// It's a resend request.
	if action == actResend {
		msg = "OTP resent"
		if err := push(out, app.providerTpls[pro.ID()], pro, app.RootURL); err != nil {
			app.logger.Printf("error sending OTP: %v", err)
			otpErr = errors.New("error resending the OTP")
		}
	}

	if otpErr != nil {
		msg = otpErr.Error()
	}

	app.tpl.ExecuteTemplate(w, "otp", tpl{App: app,
		ChannelName: pro.ChannelName(),
		MaxOTPLen:   pro.MaxOTPLen(),
		Message:     msg,
		Title:       fmt.Sprintf("Verify %s", pro.ChannelName()),
		ChannelDesc: pro.ChannelDesc(),
		AddressDesc: pro.AddressDesc(),
		OTP:         out,
	})
}

// handleAddressView renders the UI for collecting the provider address for
// verification from the user.
func handleAddressView(w http.ResponseWriter, r *http.Request) {
	var (
		app       = r.Context().Value("app").(*App)
		namespace = chi.URLParam(r, "namespace")
		id        = chi.URLParam(r, "id")
		to        = r.FormValue("to")
	)

	out, err := app.store.Check(namespace, id, false)
	if err != nil {
		if err == otpgateway.ErrNotExist {
			app.tpl.ExecuteTemplate(w, "message", tpl{App: app,
				Title: "Session expired",
				Description: `Your session has expired.
					Please re-initiate the verification.`,
			})
		} else {
			app.logger.Printf("error checking OTP: %v", err)
			app.tpl.ExecuteTemplate(w, "message", tpl{App: app,
				Title:       "Internal error",
				Description: `Please try later.`,
			})
		}
		return
	}

	// Address is already set.
	if out.To != "" {
		http.Redirect(w, r, fmt.Sprintf(uriViewOTP, out.Namespace, out.ID),
			http.StatusFound)
	}

	// Get the provider.
	pro, ok := app.providers[out.Provider]
	if !ok {
		app.tpl.ExecuteTemplate(w, "message", tpl{App: app,
			Title:       "Internal error",
			Description: "The provider for this OTP was not found.",
		})
		return
	}

	// Validate the address.
	msg := ""
	if to != "" {
		if err := pro.ValidateAddress(to); err != nil {
			msg = err.Error()
		} else if err := app.store.SetAddress(namespace, id, to); err != nil {
			msg = err.Error()
		} else {
			out.To = to
			if err := push(out, app.providerTpls[pro.ID()], pro, app.RootURL); err != nil {
				app.logger.Printf("error sending OTP: %v", err)
				msg = "error sending OTP"
			} else {
				http.Redirect(w, r, fmt.Sprintf(uriViewOTP, out.Namespace, out.ID),
					http.StatusFound)
			}
		}
	}

	app.tpl.ExecuteTemplate(w, "index", tpl{App: app,
		ChannelName:   pro.ChannelName(),
		AddressName:   pro.AddressName(),
		MaxAddressLen: pro.MaxAddressLen(),
		Message:       msg,
		Title:         fmt.Sprintf("Verify %s", pro.ChannelName()),
		ChannelDesc:   pro.ChannelDesc(),
		AddressDesc:   pro.AddressDesc(),
		OTP:           out,
	})
}

// verifyOTP validates an OTP against user input.
func verifyOTP(namespace, id, otp string, app *App) (models.OTP, error) {
	// Check the OTP.
	out, err := app.store.Check(namespace, id, true)
	if err != nil {
		if err != otpgateway.ErrNotExist {
			app.logger.Printf("error checking OTP: %v", err)
			return out, err
		}
		return out, errors.New("error checking OTP")
	}

	errMsg := ""
	if isLocked(out) {
		errMsg = fmt.Sprintf("Too many attempts. Please retry after %0.f seconds.",
			out.TTL.Seconds())
	} else if out.OTP != otp {
		errMsg = "OTP does not match"
	}

	// There was an error.
	if errMsg != "" {
		return out, errors.New(errMsg)
	}

	app.store.Close(namespace, id)
	out.Closed = true
	return out, err
}

// wrap is a middleware that wraps HTTP handlers and injects the "app" context.
func wrap(app *App, next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), "app", app)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// sendErrorResponse sends a JSON envelope to the HTTP response.
func sendResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	out, err := json.Marshal(httpResp{Status: "success", Data: data})
	if err != nil {
		sendErrorResponse(w, "Internal Server Error", http.StatusInternalServerError, nil)
		return
	}

	w.Write(out)
}

// sendErrorResponse sends a JSON error envelope to the HTTP response.
func sendErrorResponse(w http.ResponseWriter, message string, code int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)

	resp := httpResp{Status: "error",
		Message: message,
		Data:    data}
	out, _ := json.Marshal(resp)
	w.Write(out)
}

// generateRandomString generates a cryptographically random,
// alphanumeric string of length n.
func generateRandomString(totalLen int, chars string) (string, error) {
	bytes := make([]byte, totalLen)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	for k, v := range bytes {
		bytes[k] = chars[v%byte(len(chars))]
	}
	return string(bytes), nil
}

// isLocked tells if an OTP is locked after exceeding attempts.
func isLocked(otp models.OTP) bool {
	if otp.Attempts >= otp.MaxAttempts {
		return true
	}
	return false
}

// push compiles a message template and pushes it to the provider.
func push(otp models.OTP, tpl *providerTpl, p otpgateway.Provider, rootURL string) error {
	var (
		subj = &bytes.Buffer{}
		out  = &bytes.Buffer{}

		data = pushTpl{
			Channel:   p.ChannelName(),
			Namespace: otp.Namespace,
			To:        otp.To,
			OTP:       otp.OTP,
			OTPURL:    getURL(rootURL, otp, true),
		}
	)

	if tpl.subject != nil {
		if err := tpl.subject.Execute(subj, data); err != nil {
			return err
		}
	}

	if tpl.tpl != nil {
		if err := tpl.tpl.Execute(out, data); err != nil {
			return err
		}
	}

	return p.Push(otp, string(subj.Bytes()), out.Bytes())
}

func getURL(rootURL string, otp models.OTP, check bool) string {
	if check {
		return rootURL + fmt.Sprintf(uriCheck, otp.Namespace, otp.ID, otp.OTP)
	}
	return rootURL + fmt.Sprintf(uriViewOTP, otp.Namespace, otp.ID)
}

// auth is a simple authentication middleware.
func auth(authMap map[string]string, next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		const authBasic = "Basic"
		var (
			pair  [][]byte
			delim = []byte(":")

			h = r.Header.Get("Authorization")
		)

		// Basic auth scheme.
		if strings.HasPrefix(h, authBasic) {
			payload, err := base64.StdEncoding.DecodeString(string(strings.Trim(h[len(authBasic):], " ")))
			if err != nil {
				sendErrorResponse(w, "invalid Base64 value in Basic Authorization header",
					http.StatusUnauthorized, nil)
				return
			}

			pair = bytes.SplitN(payload, delim, 2)
		} else {
			sendErrorResponse(w, "missing Basic Authorization header",
				http.StatusUnauthorized, nil)
			return

		}

		if len(pair) != 2 {
			sendErrorResponse(w, "invalid value in Basic Authorization header",
				http.StatusUnauthorized, nil)
			return
		}

		var (
			namespace = string(pair[0])
			secret    = string(pair[1])
		)
		key, ok := authMap[namespace]
		if !ok || key != secret {
			sendErrorResponse(w, "invalid API credentials",
				http.StatusUnauthorized, nil)
			return
		}

		ctx := context.WithValue(r.Context(), "namespace", namespace)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
