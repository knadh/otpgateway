module github.com/knadh/otpgateway/v3

go 1.21

require (
	github.com/Masterminds/sprig v2.22.0+incompatible
	github.com/alicebob/miniredis/v2 v2.30.5
	github.com/aws/aws-sdk-go v1.45.13
	github.com/go-chi/chi/v5 v5.0.10
	github.com/knadh/koanf/parsers/toml v0.1.0
	github.com/knadh/koanf/providers/env v0.1.0
	github.com/knadh/koanf/providers/file v0.1.0
	github.com/knadh/koanf/providers/posflag v0.1.0
	github.com/knadh/koanf/v2 v2.0.1
	github.com/knadh/smtppool v1.0.2
	github.com/knadh/stuffbin v1.1.0
	github.com/redis/go-redis/v9 v9.1.0
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.8.1
	github.com/zerodha/logf v0.5.5
)

require (
	github.com/Masterminds/goutils v1.1.1 // indirect
	github.com/Masterminds/semver v1.5.0 // indirect
	github.com/alicebob/gopher-json v0.0.0-20200520072559-a9ecdc9d1d3a // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/fsnotify/fsnotify v1.6.0 // indirect
	github.com/google/uuid v1.3.1 // indirect
	github.com/huandu/xstrings v1.4.0 // indirect
	github.com/imdario/mergo v1.0.0 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/knadh/koanf/maps v0.1.1 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/pelletier/go-toml v1.9.5 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/yuin/gopher-lua v1.1.0 // indirect
	golang.org/x/crypto v0.13.0 // indirect
	golang.org/x/sys v0.12.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// https://github.com/darccio/mergo#100
replace github.com/imdario/mergo => github.com/imdario/mergo v0.3.16

replace github.com/alicebob/miniredis/v2 v2.30.5 => /home/karan/Code/OSS/miniredis
