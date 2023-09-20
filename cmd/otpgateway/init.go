package main

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Masterminds/sprig"
	"github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/posflag"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/otpgateway/v3/internal/providers/kaleyra"
	"github.com/knadh/otpgateway/v3/internal/providers/pinpoint"
	"github.com/knadh/otpgateway/v3/internal/providers/smtp"
	"github.com/knadh/otpgateway/v3/internal/providers/webhook"
	"github.com/knadh/otpgateway/v3/pkg/models"
	"github.com/zerodha/logf"

	"github.com/knadh/stuffbin"
	flag "github.com/spf13/pflag"
)

// initLogger initializes logger instance.
func initLogger(debug bool) logf.Logger {
	opts := logf.Opts{EnableCaller: true}
	if debug {
		opts.Level = logf.DebugLevel
	}
	return logf.New(opts)
}

type constants struct {
	OtpTTL         time.Duration
	OtpMaxAttempts int

	// Exported to templates.
	RootURL    string
	LogoURL    string
	FaviconURL string
}

type providerTpl struct {
	subject *template.Template
	body    *template.Template
}

type provider struct {
	provider models.Provider
	tpl      *providerTpl
}

func initConfig() {
	// Register --help handler.
	f := flag.NewFlagSet("config", flag.ContinueOnError)
	f.Usage = func() {
		fmt.Println(f.FlagUsages())
		os.Exit(0)
	}
	f.StringSlice("config", []string{"config.toml"},
		"Path to one or more TOML config files to load in order")
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
		lo.Printf("reading config: %s", f)
		if err := ko.Load(file.Provider(f), toml.Parser()); err != nil {
			lo.Printf("error reading config: %v", err)
		}
	}
	// Load environment variables and merge into the loaded config.
	if err := ko.Load(env.Provider("OTP_GATEWAY_", ".", func(s string) string {
		return strings.Replace(strings.ToLower(
			strings.TrimPrefix(s, "OTP_GATEWAY_")), "__", ".", -1)
	}), nil); err != nil {
		lo.Fatalf("error loading env config: %v", err)
	}

	ko.Load(posflag.Provider(f, ".", ko), nil)
}

// initProviders loads models.Provider plugins from the list of given filenames.
func initProviders(ko *koanf.Koanf) map[string]*provider {
	// Reserved names for in-built providers.
	bundled := map[string]bool{
		"smtp":             true,
		"pinpoint_sms":     true,
		"kaleyra_sms":      true,
		"kaleyra_whatsapp": true,
	}

	out := make(map[string]*provider)

	// Initialized the in-built providers.
	// SMTP.
	if ko.Bool("providers.smtp.enabled") {
		var cfg smtp.Config
		if err := ko.UnmarshalWithConf("providers.smtp", &cfg, koanf.UnmarshalConf{Tag: "json"}); err != nil {
			lo.Fatalf("error unmarshalling providers.smtp config: %v", err)
		}

		p, err := smtp.New(cfg)
		if err != nil {
			lo.Fatalf("error initializing smtp provider: %v", err)
		}

		out["smtp"] = &provider{
			provider: p,
			tpl:      initProviderTpl(ko.String("providers.smtp.subject"), ko.String("providers.smtp.template")),
		}
	}

	// Pinpoint SMS.
	if ko.Bool("providers.pinpoint_sms.enabled") {
		var cfg pinpoint.Config
		if err := ko.UnmarshalWithConf("providers.pinpoint_sms", &cfg, koanf.UnmarshalConf{Tag: "json"}); err != nil {
			lo.Fatalf("error unmarshalling providers.pinpoint_sms config: %v", err)
		}

		p, err := pinpoint.NewSMS(cfg)
		if err != nil {
			lo.Fatalf("error initializing pinpoint provider: %v", err)
		}

		out["pinpoint_sms"] = &provider{
			provider: p,
			tpl:      initProviderTpl(ko.String("providers.pinpoint_sms.subject"), ko.String("providers.pinpoint_sms.template")),
		}
	}

	// Kaleyra.
	for _, k := range []string{"kaleyra_sms", "kaleyra_whatsapp"} {
		if !ko.Bool(fmt.Sprintf("providers.%s.enabled", k)) {
			continue
		}

		var cfg kaleyra.Config
		if err := ko.UnmarshalWithConf(fmt.Sprintf("providers.%s", k), &cfg, koanf.UnmarshalConf{Tag: "json"}); err != nil {
			lo.Fatalf("error unmarshalling providers.%s config: %v", k, err)
		}

		typ := kaleyra.ChannelSMS
		if k == "kaleyra_whatsapp" {
			typ = kaleyra.ChannelWhatsapp
		}

		p, err := kaleyra.New(typ, cfg)
		if err != nil {
			lo.Fatalf("error initializing %s provider: %v", k, err)
		}

		out[k] = &provider{
			provider: p,
			tpl:      initProviderTpl(ko.String(fmt.Sprintf("providers.%s.subject", k)), ko.String(fmt.Sprintf("providers.%s.template", k))),
		}
	}

	// Load custom webhook providers.
	for _, name := range ko.MapKeys("webhooks") {
		if _, ok := bundled[name]; ok {
			lo.Fatalf("webhook name '%s' is reserved in providers.'%s'", name, name)
		}

		key := fmt.Sprintf("webhooks.%s", name)

		if !ko.Bool(fmt.Sprintf("%s.enabled", key)) {
			continue
		}

		var cfg webhook.Config
		if err := ko.UnmarshalWithConf(key, &cfg, koanf.UnmarshalConf{Tag: "json"}); err != nil {
			lo.Fatalf("error unmarshalling %s config: %v", key, err)
		}

		p, err := webhook.New(cfg)
		if err != nil {
			lo.Fatalf("error initializing %s: %v", key, err)
		}
		out[name] = &provider{
			provider: p,
			tpl:      initProviderTpl(ko.String(fmt.Sprintf("%s.subject", key)), ko.String(fmt.Sprintf("%s.template", key))),
		}
	}

	if len(out) == 0 {
		lo.Fatal("no providers or webhooks enabled")
	}

	names := []string{}
	for name := range out {
		names = append(names, name)
	}

	lo.Printf("enabled providers: %s", strings.Join(names, ", "))

	return out
}

// initAuth loads the namespace:token authorisation maps.
func initAuth() map[string]string {
	out := make(map[string]string)
	for _, a := range ko.MapKeys("auth") {
		k := ko.StringMap("auth." + a)
		var (
			namespace, _ = k["namespace"]
			secret, _    = k["secret"]
		)

		if namespace == "" || secret == "" {
			lo.Fatalf("namespace or secret keys not found in auth.%s", a)
		}
		out[k["namespace"]] = k["secret"]
	}

	return out
}

// initProviderTpl loads a provider's optional templates.
func initProviderTpl(subj, tplFile string) *providerTpl {
	out := &providerTpl{}

	// Template file.
	if tplFile != "" {
		// Parse the template file.
		// tpl, err := template.ParseFiles(tplFile)

		tpl, err := template.New(filepath.Base(tplFile)).Funcs(sprig.FuncMap()).ParseFiles(tplFile)

		if err != nil {
			lo.Fatalf("error parsing template file: %s: %v", tplFile, err)
		}
		out.body = tpl
	}

	// Subject template string.
	if subj != "" {
		tpl, err := template.New("subject").Parse(subj)
		if err != nil {
			lo.Fatalf("error parsing template subject: %s: %v", tplFile, err)
		}

		out.subject = tpl
	}

	return out
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
				lo.Fatalf("error falling back to local filesystem: %v", err)
			}
		} else {
			lo.Fatalf("error reading stuffed binary: %v", err)
		}
	}

	return fs
}
