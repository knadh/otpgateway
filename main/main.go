package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"plugin"
	"strings"
	"time"

	"github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/posflag"

	"github.com/go-chi/chi"
	"github.com/knadh/koanf"
	"github.com/knadh/otpgateway"
	"github.com/knadh/stuffbin"
	flag "github.com/spf13/pflag"
)

type providerTpl struct {
	subject *template.Template
	tpl     *template.Template
}

// App is the global app context that groups the necessary
// controls (db, config etc.) to be injected into the HTTP handlers.
type App struct {
	store        otpgateway.Store
	providers    map[string]otpgateway.Provider
	providerTpls map[string]*providerTpl
	logger       *log.Logger
	tpl          *template.Template
	fs           stuffbin.FileSystem

	// Constants
	otpTTL         time.Duration
	otpMaxAttempts int

	// Exported to templates.
	RootURL    string
	LogoURL    string
	FaviconURL string
}

var (
	logger = log.New(os.Stdout, "", log.Ldate|log.Ltime|log.Lshortfile)
	ko     = koanf.New(".")

	// Version of the build injected at build time.
	buildString = "unknown"
)

func initConfig() {
	// Register --help handler.
	f := flag.NewFlagSet("config", flag.ContinueOnError)
	f.Usage = func() {
		fmt.Println(f.FlagUsages())
		os.Exit(0)
	}
	f.StringSlice("config", []string{"config.toml"},
		"Path to one or more TOML config files to load in order")
	f.StringSlice("prov", []string{"smtp.prov"},
		"Path to a provider plugin. Can specify multiple values.")
	f.Bool("version", false, "Show build version")
	f.Parse(os.Args[1:])

	// Display version.
	if ok, _ := f.GetBool("version"); ok {
		fmt.Println(buildString)
		os.Exit(0)
	}

	// Read the config files.
	cFiles, _ := f.GetStringSlice("config")
	for _, f := range cFiles {
		log.Printf("reading config: %s", f)
		if err := ko.Load(file.Provider(f), toml.Parser()); err != nil {
			log.Printf("error reading config: %v", err)
		}
	}
	// Load environment variables and merge into the loaded config.
	if err := ko.Load(env.Provider("OTP_GATEWAY_", ".", func(s string) string {
		return strings.Replace(strings.ToLower(
			strings.TrimPrefix(s, "OTP_GATEWAY_")), "__", ".", -1)
	}), nil); err != nil {
		log.Printf("error loading env config: %v", err)
	}

	ko.Load(posflag.Provider(f, ".", ko), nil)
}

// loadProviders loads otpgateway.Provider plugins from the list of given filenames.
func loadProviders(names []string) (map[string]otpgateway.Provider, error) {
	out := make(map[string]otpgateway.Provider)
	for _, fName := range names {
		plg, err := plugin.Open(fName)
		if err != nil {
			return nil, fmt.Errorf("error loading provider plugin '%s': %v", fName, err)
		}
		id := strings.TrimSuffix(filepath.Base(fName), filepath.Ext(fName))

		newFunc, err := plg.Lookup("New")
		if err != nil {
			return nil, fmt.Errorf("New() function not found in plugin '%s': %v", id, err)
		}
		f, ok := newFunc.(func([]byte) (interface{}, error))
		if !ok {
			return nil, fmt.Errorf("New() function is of invalid type (%T) in plugin '%s'", newFunc, id)
		}

		// Plugin loaded. Load it's configuration.
		var cfg otpgateway.ProviderConf
		ko.Unmarshal("provider."+id, &cfg)
		if cfg.Config == "" {
			logger.Printf("WARNING: No config 'provider.%s' for '%s' in config", id, id)
		}

		// Initialize the plugin.
		provider, err := f([]byte(cfg.Config))
		if err != nil {
			return nil, fmt.Errorf("error initializing provider plugin '%s': %v", id, err)
		}
		logger.Printf("loaded provider plugin '%s' from %s", id, fName)

		p, ok := provider.(otpgateway.Provider)
		if !ok {
			return nil, fmt.Errorf("New() function does not return a provider that satisfies otpgateway.Provider (%T) in plugin '%s'", provider, id)
		}

		if p.ID() != id {
			return nil, fmt.Errorf("provider plugin ID doesn't match '%s' != %s", id, p.ID())
		}
		out[p.ID()] = p
	}
	return out, nil
}

// loadAuth loads the namespace:token authorisation maps.
func loadAuth() map[string]string {
	out := make(map[string]string)
	for _, a := range ko.MapKeys("auth") {
		k := ko.StringMap("auth." + a)
		var (
			namespace, _ = k["namespace"]
			secret, _    = k["secret"]
		)

		if namespace == "" || secret == "" {
			logger.Fatalf("namespace or secret keys not found in auth.%s", a)
		}
		out[k["namespace"]] = k["secret"]
	}
	return out
}

// loadProviderTemplates loads a provider's templates.
func loadProviderTemplates(providers []string) (map[string]*providerTpl, error) {
	out := make(map[string]*providerTpl)
	for _, p := range providers {
		var (
			tplFile      = ko.String(fmt.Sprintf("provider.%s.template", p))
			subj         = ko.String(fmt.Sprintf("provider.%s.subject", p))
			tpl, subjTpl *template.Template
			err          error
		)
		// Optional template and subject file.
		if tplFile != "" {
			// Parse the template file.
			tpl, err = template.ParseFiles(tplFile)
			if err != nil {
				return nil, fmt.Errorf("error parsing template %s for %s: %v", tplFile, p, err)
			}
		}
		if subj != "" {
			subjTpl, err = template.New("subject").Parse(subj)
			if err != nil {
				return nil, fmt.Errorf("error parsing template %s: %v", p, err)
			}
		}

		out[p] = &providerTpl{
			subject: subjTpl,
			tpl:     tpl,
		}
	}
	return out, nil
}

func initFS(exe string) stuffbin.FileSystem {
	// Read stuffed data from self.
	fs, err := stuffbin.UnStuff(exe)
	if err != nil {
		// Binary is unstuffed or is running in dev mode.
		// Can halt here or fall back to the local filesystem.
		if err == stuffbin.ErrNoID {
			// First argument is to the root to mount the files in the FileSystem
			// and the rest of the arguments are paths to embed.
			fs, err = stuffbin.NewLocalFS("/", "static/")
			if err != nil {
				log.Fatalf("error falling back to local filesystem: %v", err)
			}
		} else {
			log.Fatalf("error reading stuffed binary: %v", err)
		}
	}
	return fs
}

func main() {
	initConfig()

	app := &App{}
	provs, err := loadProviders(ko.Strings("prov"))
	if err != nil {
		logger.Fatal(err)
	} else if len(provs) == 0 {
		logger.Fatal("no providers loaded. Use --provider to load a provider plugin.")
	}

	app.providers = provs
	app.logger = logger
	app.otpTTL = ko.Duration("app.otp_ttl") * time.Second
	app.otpMaxAttempts = ko.Int("app.otp_max_attempts")
	app.RootURL = strings.TrimRight(ko.String("app.root_url"), "/")
	app.LogoURL = ko.String("app.logo_url")
	app.FaviconURL = ko.String("app.favicon_url")
	app.fs = initFS(os.Args[0])

	// Load provider templates.
	var pNames []string
	for p := range provs {
		pNames = append(pNames, p)
	}
	tpls, err := loadProviderTemplates(pNames)
	if err != nil {
		logger.Fatal(err)
	}
	app.providerTpls = tpls

	// Load the store.
	var rc otpgateway.RedisConf
	ko.Unmarshal("store.redis", &rc)
	app.store = otpgateway.NewRedisStore(rc)

	// Compile static templates.
	tpl, err := stuffbin.ParseTemplatesGlob(nil, app.fs, "/static/*.html")
	if err != nil {
		logger.Fatalf("error compiling template: %v", err)
	}
	app.tpl = tpl

	authCreds := loadAuth()
	if len(authCreds) == 0 {
		logger.Fatal("no auth entries found in config")
	}

	// Register handles.
	r := chi.NewRouter()
	r.Get("/api/providers", auth(authCreds, wrap(app, handleGetProviders)))
	r.Get("/api/health", wrap(app, handleHealthCheck))
	r.Put("/api/otp/{id}", auth(authCreds, wrap(app, handleSetOTP)))
	r.Post("/api/otp/{id}/status", auth(authCreds, wrap(app, handleCheckOTPStatus)))
	r.Post("/api/otp/{id}", auth(authCreds, wrap(app, handleVerifyOTP)))

	r.Get("/otp/{namespace}/{id}", wrap(app, handleOTPView))
	r.Get("/otp/{namespace}/{id}/address", wrap(app, handleAddressView))
	r.Post("/otp/{namespace}/{id}/address", wrap(app, handleAddressView))
	r.Post("/otp/{namespace}/{id}", wrap(app, handleOTPView))
	r.Get("/static/*", func(w http.ResponseWriter, r *http.Request) {
		app.fs.FileServer().ServeHTTP(w, r)
	})

	// HTTP Server.
	srv := &http.Server{
		Addr:         ko.String("app.address"),
		ReadTimeout:  ko.Duration("ap.timeout") * time.Second,
		WriteTimeout: ko.Duration("ap.timeout") * time.Second,
		Handler:      r,
	}

	logger.Printf("starting on %s", srv.Addr)
	if err := srv.ListenAndServe(); err != nil {
		logger.Fatalf("couldn't start server: %v", err)
	}
}
