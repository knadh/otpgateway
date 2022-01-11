LAST_COMMIT := $(shell git rev-parse --short HEAD)
LAST_COMMIT_DATE := $(shell git show -s --format=%ci ${LAST_COMMIT})
VERSION := $(shell git describe --abbrev=1)
BUILDSTR := ${VERSION} (build "\\\#"${LAST_COMMIT} $(shell date '+%Y-%m-%d %H:%M:%S'))

BIN := otpgateway
STATIC := static/

.PHONY: build
build:
	# Compile the main application.
	go build -o ${BIN} -ldflags="-s -w -X 'main.buildString=${BUILDSTR}'" cmd/otpgateway/*.go
	stuffbin -a stuff -in ${BIN} -out ${BIN} ${STATIC}

.PHONY: deps
deps:
	go get -u github.com/knadh/stuffbin/...

.PHONY: test
test:
	go test ./...

clean:
	go clean
	- rm -f ${BIN} ${SMTP_BIN}

# pack-releases runns stuffbin packing on the given binary. This is used
# in the .goreleaser post-build hook.
.PHONY: pack-bin
pack-bin: build $(BIN) $(STUFFBIN)
	$(STUFFBIN) -a stuff -in ${BIN} -out ${BIN} ${STATIC}
