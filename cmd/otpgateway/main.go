package main

import (
	"html/template"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi"
	"github.com/knadh/koanf"
	"github.com/knadh/otpgateway/v3/internal/store"
	"github.com/knadh/otpgateway/v3/internal/store/redis"
	"github.com/knadh/otpgateway/v3/internal/models"
	"github.com/knadh/stuffbin"
)

type providerTpl struct {
	subject *template.Template
	tpl     *template.Template
}

// App is the global app context that groups the necessary
// controls (db, config etc.) to be injected into the HTTP handlers.
type App struct {
	store        store.Store
	providers    map[string]models.Provider
	providerTpls map[string]*providerTpl
	log          *log.Logger
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

	provs, err := initProviders(ko.Strings("prov"))
	if err != nil {
		lo.Fatal(err)
	} else if len(provs) == 0 {
		lo.Fatal("no providers loaded. Use --provider to load a provider plugin.")
	}

	app := &App{
		providers: provs,
		log:       lo,
		fs:        initFS(os.Args[0]),

		constants: constants{
			OtpTTL:         ko.Duration("app.otp_ttl") * time.Second,
			OtpMaxAttempts: ko.Int("app.otp_max_attempts"),
			RootURL:        strings.TrimRight(ko.String("app.root_url"), "/"),
			LogoURL:        ko.String("app.logo_url"),
			FaviconURL:     ko.String("app.favicon_url"),
		},
	}

	// Load provider templates.
	var pNames []string
	for p := range provs {
		pNames = append(pNames, p)
	}
	tpls, err := initProviderTemplates(pNames)
	if err != nil {
		lo.Fatal(err)
	}
	app.providerTpls = tpls

	// Load the store.
	var rc redis.Conf
	ko.UnmarshalWithConf("store.redis", &rc, koanf.UnmarshalConf{Tag: "json"})
	app.store = redis.New(rc)

	// Compile static templates.
	tpl, err := stuffbin.ParseTemplatesGlob(nil, app.fs, "/static/*.html")
	if err != nil {
		lo.Fatalf("error compiling template: %v", err)
	}
	app.tpl = tpl

	authCreds := initAuth()
	if len(authCreds) == 0 {
		lo.Fatal("no auth entries found in config")
	}

	// Register handles.
	r := chi.NewRouter()
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("otpgateway"))
	})
	r.Get("/api/providers", auth(authCreds, wrap(app, handleGetProviders)))
	r.Get("/api/health", wrap(app, handleHealthCheck))
	r.Put("/api/otp/{id}", auth(authCreds, wrap(app, handleSetOTP)))
	r.Post("/api/otp/{id}/status", auth(authCreds, wrap(app, handleCheckOTPStatus)))
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
	timeout := ko.Duration("app.server_timeout")
	if timeout.Seconds() < 1 {
		timeout = time.Second * 5
	}

	srv := &http.Server{
		Addr:         ko.String("app.address"),
		ReadTimeout:  timeout,
		WriteTimeout: timeout,
		Handler:      r,
	}

	lo.Printf("starting on %s", srv.Addr)
	if err := srv.ListenAndServe(); err != nil {
		lo.Fatalf("couldn't start server: %v", err)
	}
}
