LAST_COMMIT := $(shell git rev-parse --short HEAD)
LAST_COMMIT_DATE := $(shell git show -s --format=%ci ${LAST_COMMIT})
VERSION := $(shell git describe --abbrev=1)
BUILDSTR := ${VERSION} (build "\\\#"${LAST_COMMIT} $(shell date '+%Y-%m-%d %H:%M:%S'))

BIN := otpgateway
SMTP_BIN := smtp.prov
STATIC := static/

.PHONY: build
build:
	# Compile the smtp provider plugin.
	go build -ldflags="-s -w" -buildmode=plugin -linkshared -o ${SMTP_BIN} providers/smtp/smtp.go

	# Compile the main application.
	go build -o ${BIN} -ldflags="-s -w -X 'main.buildString=${BUILDSTR}'" main/*.go
	stuffbin -a stuff -in ${BIN} -out ${BIN} ${STATIC}

.PHONY: deps
deps:
	go get -u github.com/knadh/stuffbin/...

.PHONY: test
test:
	go test

clean:
	go clean
	- rm -f ${BIN} ${SMTP_BIN}
