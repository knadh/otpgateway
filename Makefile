HASH := $(shell git rev-parse --short HEAD)
VER := $(shell git describe --abbrev=0)
COMMIT_DATE := $(shell git show -s --format=%ci ${HASH})
BUILD := (${HASH}) $(shell date '+%Y-%m-%d %H:%M:%S')

.PHONY: build
build:
	# Compile the smtp provider plugin.
	go build -ldflags="-s -w" -buildmode=plugin -linkshared -o smtp.prov providers/smtp/smtp.go

	# Compile the main application.
	go build -o otpgateway -ldflags="-s -w -X 'main.buildVersion=${VER}' -X 'main.buildDate=${BUILD}'" main/*.go
