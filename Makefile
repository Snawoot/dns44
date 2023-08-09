PROGNAME = dns44
OUTSUFFIX = bin/$(PROGNAME)
VERSION := $(shell git describe)
BUILDOPTS = -a -tags netgo
LDFLAGS = -ldflags '-s -w -extldflags "-static" -X main.version=$(VERSION)'
LDFLAGS_NATIVE = -ldflags '-s -w -X main.version=$(VERSION)'
MAIN_PACKAGE = ./cmd/$(PROGNAME)

NDK_CC_ARM = $(abspath ../../ndk-toolchain-arm/bin/arm-linux-androideabi-gcc)
NDK_CC_ARM64 = $(abspath ../../ndk-toolchain-arm64/bin/aarch64-linux-android21-clang)

GO := go
GOIMPORTS := goimports

src = $(wildcard *.go */*.go */*/*.go) go.mod go.sum

native: bin-native
all: bin-linux-amd64 bin-linux-386 bin-linux-arm bin-linux-arm64 \
	bin-linux-mips bin-linux-mipsle bin-linux-mips64 bin-linux-mips64le

bin-native: $(OUTSUFFIX)
bin-linux-amd64: $(OUTSUFFIX).linux-amd64
bin-linux-386: $(OUTSUFFIX).linux-386
bin-linux-arm: $(OUTSUFFIX).linux-arm
bin-linux-arm64: $(OUTSUFFIX).linux-arm64
bin-linux-mips: $(OUTSUFFIX).linux-mips
bin-linux-mipsle: $(OUTSUFFIX).linux-mipsle
bin-linux-mips64: $(OUTSUFFIX).linux-mips64
bin-linux-mips64le: $(OUTSUFFIX).linux-mips64le

$(OUTSUFFIX): $(src)
	$(GO) build $(LDFLAGS_NATIVE) -o $@ $(MAIN_PACKAGE)

$(OUTSUFFIX).linux-amd64: $(src)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build $(BUILDOPTS) $(LDFLAGS) -o $@ $(MAIN_PACKAGE)

$(OUTSUFFIX).linux-386: $(src)
	CGO_ENABLED=0 GOOS=linux GOARCH=386 $(GO) build $(BUILDOPTS) $(LDFLAGS) -o $@ $(MAIN_PACKAGE)

$(OUTSUFFIX).linux-arm: $(src)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm $(GO) build $(BUILDOPTS) $(LDFLAGS) -o $@ $(MAIN_PACKAGE)

$(OUTSUFFIX).linux-arm64: $(src)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build $(BUILDOPTS) $(LDFLAGS) -o $@ $(MAIN_PACKAGE)

$(OUTSUFFIX).linux-mips: $(src)
	CGO_ENABLED=0 GOOS=linux GOARCH=mips GOMIPS=softfloat $(GO) build $(BUILDOPTS) $(LDFLAGS) -o $@ $(MAIN_PACKAGE)

$(OUTSUFFIX).linux-mips64: $(src)
	CGO_ENABLED=0 GOOS=linux GOARCH=mips64 GOMIPS=softfloat $(GO) build $(BUILDOPTS) $(LDFLAGS) -o $@ $(MAIN_PACKAGE)

$(OUTSUFFIX).linux-mipsle: $(src)
	CGO_ENABLED=0 GOOS=linux GOARCH=mipsle GOMIPS=softfloat $(GO) build $(BUILDOPTS) $(LDFLAGS) -o $@ $(MAIN_PACKAGE)

$(OUTSUFFIX).linux-mips64le: $(src)
	CGO_ENABLED=0 GOOS=linux GOARCH=mips64le GOMIPS=softfloat $(GO) build $(BUILDOPTS) $(LDFLAGS) -o $@ $(MAIN_PACKAGE)

clean:
	rm -f bin/*

fmt:
	$(GOIMPORTS) -w .

run:
	$(GO) run $(LDFLAGS) $(MAIN_PACKAGE)

install:
	$(GO) install $(LDFLAGS_NATIVE) $(MAIN_PACKAGE)

.PHONY: clean all native fmt install \
	bin-native \
	bin-linux-amd64 \
	bin-linux-386 \
	bin-linux-arm \
	bin-linux-arm64 \
	bin-linux-mips \
	bin-linux-mipsle \
	bin-linux-mips64 \
	bin-linux-mips64le \
	bin-freebsd-amd64 \
	bin-freebsd-386 \
	bin-freebsd-arm \
	bin-freebsd-arm64 \
	bin-netbsd-amd64 \
	bin-netbsd-386 \
	bin-netbsd-arm \
	bin-netbsd-arm64 \
	bin-openbsd-amd64 \
	bin-openbsd-386 \
	bin-openbsd-arm \
	bin-openbsd-arm64 \
	bin-darwin-amd64 \
	bin-darwin-arm64 \
	bin-windows-amd64 \
	bin-windows-386 \
	bin-windows-arm \
	bin-android-arm \
	bin-android-arm64
