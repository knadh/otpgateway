package main

import (
	"html/template"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/otpgateway/v3/internal/store"
	"github.com/knadh/otpgateway/v3/internal/store/redis"
	"github.com/knadh/stuffbin"
	"github.com/zerodha/logf"
)

// App is the global app context that groups the necessary
// controls (db, config etc.) to be injected into the HTTP handlers.
type App struct {
	store        store.Store
	providers    map[string]*provider
	providerTpls map[string]*providerTpl
	lo           logf.Logger
	tpl          *template.Template
	fs           stuffbin.FileSystem
	constants    constants
}

var (
	lo = log.New(os.Stdout, "", log.Ldate|log.Ltime|log.Lshortfile)
	ko = koanf.New(".")

	// Version of the build injected at build time.
	buildString = "unknown"
)

func main() {
	initConfig()

	app := &App{
		fs:        initFS(os.Args[0]),
		providers: initProviders(ko),
		lo:        initLogger(ko.Bool("app.enable_debug_logs")),

		constants: constants{
			OtpTTL:         ko.MustDuration("app.otp_ttl") * time.Second,
			OtpMaxAttempts: ko.MustInt("app.otp_max_attempts"),
			RootURL:        strings.TrimRight(ko.String("app.root_url"), "/"),
			LogoURL:        ko.String("app.logo_url"),
			FaviconURL:     ko.String("app.favicon_url"),
		},
	}

	// Initialize the Redis store.
	var rc redis.Conf
	ko.UnmarshalWithConf("store.redis", &rc, koanf.UnmarshalConf{Tag: "json"})
	app.store = redis.New(rc)

	// Check if the Redis server is available by sending a Ping.
	if err := app.store.Ping(); err != nil {
		log.Fatalf("failed to connect to redis: %v", err)
	}

	// Compile static templates.
	tpl, err := stuffbin.ParseTemplatesGlob(nil, app.fs, "/static/*.html")
	if err != nil {
		app.lo.Fatal("error compiling template", "error", err)
	}
	app.tpl = tpl

	authCreds := initAuth()
	if len(authCreds) == 0 {
		app.lo.Fatal("no auth entries found in config")
	}

	// Register HTTP handlers.
	r := chi.NewRouter()
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("otpgateway"))
	})
	r.Get("/api/providers", auth(authCreds, wrap(app, handleGetProviders)))
	r.Get("/api/health", wrap(app, handleHealthCheck))
	r.Put("/api/otp/{id}", auth(authCreds, wrap(app, handleSetOTP)))
	r.Post("/api/otp/{id}/status", auth(authCreds, wrap(app, handleCheckOTPStatus)))
	r.Delete("/api/otp/{id}/status", auth(authCreds, wrap(app, handleCheckOTPStatus)))
	r.Post("/api/otp/{id}", auth(authCreds, wrap(app, handleVerifyOTP)))

	r.Get("/otp/{namespace}/{id}", wrap(app, handleOTPView))
	r.Get("/otp/{namespace}/{id}/status", wrap(app, handleGetOTPClosed))
	r.Get("/otp/{namespace}/{id}/address", wrap(app, handleAddressView))
	r.Post("/otp/{namespace}/{id}/address", wrap(app, handleAddressView))
	r.Post("/otp/{namespace}/{id}", wrap(app, handleOTPView))
	r.Get("/static/*", func(w http.ResponseWriter, r *http.Request) {
		app.fs.FileServer().ServeHTTP(w, r)
	})

	// HTTP Server.

	srv := &http.Server{
		Addr:         ko.MustString("app.address"),
		ReadTimeout:  ko.MustDuration("app.server_timeout"),
		WriteTimeout: ko.MustDuration("app.server_timeout"),
		Handler:      r,
	}

	app.lo.Info("starting server", "address", srv.Addr)
	if err := srv.ListenAndServe(); err != nil {
		app.lo.Fatal("couldn't start server", "error", err)
	}
}
