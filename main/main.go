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

	"github.com/knadh/stuffbin"

	"github.com/go-chi/chi"
	"github.com/knadh/otpgateway"
	flag "github.com/spf13/pflag"
	"github.com/spf13/viper"
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

	// Version of the build injected at build time.
	buildVersion = "unknown"
	buildDate    = "unknown"
)

func initConfig() {
	// Register --help handler.
	flagSet := flag.NewFlagSet("config", flag.ContinueOnError)
	flagSet.Usage = func() {
		fmt.Println(flagSet.FlagUsages())
		os.Exit(0)
	}

	// Setup the default configuration.
	viper.SetConfigName("config")
	viper.SetDefault("app.otp_max_attempts", 5)
	viper.SetDefault("app.otp_ttl", 5)
	flagSet.StringSlice("config", []string{"config.toml"},
		"Path to one or more config files (will be merged in order)")
	flagSet.StringSlice("provider", []string{"smtp.prov"},
		"Path to a provider plugin. Can specify multiple values.")
	flagSet.Bool("version", false, "Current version of the build")

	// Process flags.
	flagSet.Parse(os.Args[1:])
	viper.BindPFlags(flagSet)

	// Read the config files.
	cfgs := viper.GetStringSlice("config")
	for _, c := range cfgs {
		viper.SetConfigFile(c)

		if err := viper.MergeInConfig(); err != nil {
			logger.Fatalf("error reading config: %s", err)
		}
	}
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
		f, ok := newFunc.(func([]byte) (otpgateway.Provider, error))
		if !ok {
			return nil, fmt.Errorf("New() function is of invalid type in plugin '%s'", id)
		}

		// Plugin loaded. Load it's configuration.
		var cfg otpgateway.ProviderConf
		viper.UnmarshalKey("provider."+id, &cfg)
		if cfg.Template == "" || cfg.Config == "" {
			logger.Printf("WARNING: No config 'provider.%s' for '%s' in config", id, id)
		}

		// Initialize the plugin.
		p, err := f([]byte(cfg.Config))
		if err != nil {
			return nil, fmt.Errorf("error initializing provider plugin '%s': %v", id, err)
		}
		logger.Printf("loaded provider plugin '%s' from %s", id, fName)

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
	for i := range viper.GetStringMapString("auth") {
		k := viper.GetStringMapString("auth." + i)
		var (
			namespace, _ = k["namespace"]
			secret, _    = k["secret"]
		)

		if namespace == "" || secret == "" {
			logger.Fatalf("namespace or secret keys not found in auth.%s", i)
		}
		out[k["namespace"]] = k["secret"]
	}

	return out
}

// loadProviderTemplates loads a provider's templates.
func loadProviderTemplates(providers []string) (map[string]*providerTpl, error) {
	out := make(map[string]*providerTpl)
	for _, p := range providers {
		tplFile := viper.GetString(fmt.Sprintf("provider.%s.template", p))
		if tplFile == "" {
			return nil, fmt.Errorf("no 'template' value found for 'provider.%s' in config", p)
		}

		// Parse the template file.
		tpl, err := template.ParseFiles(tplFile)
		if err != nil {
			return nil, fmt.Errorf("error parsing template %s for %s: %v", tplFile, p, err)
		}

		// Parse the subject.
		subj := viper.GetString(fmt.Sprintf("provider.%s.subject", p))
		if subj == "" {
			return nil, fmt.Errorf("error parsing subject for %s: %v", p, err)
		}

		subjTpl, err := template.New("subject").Parse(subj)
		if err != nil {
			return nil, fmt.Errorf("error parsing template %s: %v", p, err)
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

	// Display version.
	if viper.GetBool("version") {
		fmt.Printf("%v\nBuild: %v", buildVersion, buildDate)
		return
	}

	app := &App{}
	provs, err := loadProviders(viper.GetStringSlice("provider"))
	if err != nil {
		logger.Fatal(err)
	} else if len(provs) == 0 {
		logger.Fatal("no providers loaded. Use --provider to load a provider plugin.")
	}

	app.providers = provs
	app.logger = logger
	app.otpTTL = viper.GetDuration("app.otp_ttl") * time.Second
	app.otpMaxAttempts = viper.GetInt("app.otp_max_attempts")
	app.RootURL = strings.TrimRight(viper.GetString("app.root_url"), "/")
	app.LogoURL = viper.GetString("app.logo_url")
	app.FaviconURL = viper.GetString("app.favicon_url")
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
	viper.UnmarshalKey("store.redis", &rc)
	app.store = otpgateway.NewRedisStore(rc)

	// Compile static templates.
	tpl, err := stuffbin.ParseTemplatesGlob(app.fs, "/static/*.html")
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
		Addr:         viper.GetString("app.address"),
		ReadTimeout:  viper.GetDuration("ap.timeout") * time.Second,
		WriteTimeout: viper.GetDuration("ap.timeout") * time.Second,
		Handler:      r,
	}

	logger.Printf("starting on %s", srv.Addr)
	if err := srv.ListenAndServe(); err != nil {
		logger.Fatalf("couldn't start server: %v", err)
	}
}
