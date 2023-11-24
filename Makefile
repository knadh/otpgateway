LAST_COMMIT := $(shell git rev-parse --short HEAD)
LAST_COMMIT_DATE := $(shell git show -s --format=%ci ${LAST_COMMIT})
VERSION := $(shell git describe --abbrev=1)
BUILDSTR := ${VERSION} (build "\\\#"${LAST_COMMIT} $(shell date '+%Y-%m-%d %H:%M:%S'))

GOPATH ?= $(HOME)/go
STUFFBIN ?= $(GOPATH)/bin/stuffbin

BIN := otpgateway.bin
STATIC := static/

$(STUFFBIN):
	go install github.com/knadh/stuffbin/...

.PHONY: build
build: $(BIN)

$(BIN): $(shell find . -type f -name "*.go")
	CGO_ENABLED=0 go build -o ${BIN} -ldflags="-s -w -X 'main.buildString=${BUILDSTR}'" ./cmd/otpgateway/*.go

.PHONY: run
run:
	CGO_ENABLED=0 go run -ldflags="-s -w -X 'main.buildString=${BUILDSTR}'" ./cmd/otpgateway

.PHONY: dist
dist: $(STUFFBIN) build pack-bin

.PHONY: test
test:
	go test ./...

# pack-releases runns stuffbin packing on the given binary. This is used
# in the .goreleaser post-build hook.
.PHONY: pack-bin
pack-bin: build $(BIN) $(STUFFBIN)
	$(STUFFBIN) -a stuff -in ${BIN} -out ${BIN} ${STATIC}

# Use goreleaser to do a dry run producing local builds.
.PHONY: release-dry
release-dry:
	goreleaser --parallelism 1 --rm-dist --snapshot --skip-validate --skip-publish

# Use goreleaser to build production releases and publish them.
.PHONY: release
release:
	goreleaser --parallelism 1 --rm-dist --skip-validate
