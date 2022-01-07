package main

import (
	"fmt"
	"html/template"
	"log"
	"os"
	"path/filepath"
	"plugin"
	"strings"
	"time"

	"github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/posflag"
	"github.com/knadh/otpgateway/v3/internal/models"

	"github.com/knadh/stuffbin"
	flag "github.com/spf13/pflag"
)

type constants struct {
	OtpTTL         time.Duration
	OtpMaxAttempts int

	// Exported to templates.
	RootURL    string
	LogoURL    string
	FaviconURL string
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

// initProviders loads models.Provider plugins from the list of given filenames.
func initProviders(names []string) (map[string]models.Provider, error) {
	out := make(map[string]models.Provider)
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
		var cfg models.ProviderConfig
		ko.Unmarshal("provider."+id, &cfg)
		if cfg.Config == "" {
			lo.Printf("WARNING: No config 'provider.%s' for '%s' in config", id, id)
		}

		// Initialize the plugin.
		prov, err := f([]byte(cfg.Config))
		if err != nil {
			return nil, fmt.Errorf("error initializing provider plugin '%s': %v", id, err)
		}
		lo.Printf("loaded provider plugin '%s' from %s", id, fName)

		p, ok := prov.(models.Provider)
		if !ok {
			return nil, fmt.Errorf("New() function does not return a provider that satisfies models.Provider (%T) in plugin '%s'", prov, id)
		}

		if p.ID() != id {
			return nil, fmt.Errorf("provider plugin ID doesn't match '%s' != %s", id, p.ID())
		}
		out[p.ID()] = p
	}

	return out, nil
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

// initProviderTemplates loads a provider's templates.
func initProviderTemplates(providers []string) (map[string]*providerTpl, error) {
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
