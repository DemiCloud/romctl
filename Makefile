# Project metadata
NAME    := romctl
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

# Linker flags
LDFLAGS         := -X github.com/demicloud/romctl/internal/version.Version=$(VERSION)
RELEASE_LDFLAGS := $(LDFLAGS) -s -w

# Target platforms for `make release`
PLATFORMS := linux/amd64 linux/arm64 windows/amd64 darwin/amd64 darwin/arm64

.PHONY: build test vet release clean help tools tools-windows-amd64 fetch-mame fetch-dolphin distclean

.DEFAULT_GOAL := help

build: ## Development build → build/romctl (debug symbols; version = "dev" if no tags)
	mkdir -p build
	go mod tidy
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o build/$(NAME) ./cmd/romctl

test: ## Run the test suite
	go test -count=1 ./...

vet: ## Run go vet static analysis
	go vet ./...

release: clean ## Stripped, trimpath, multi-arch release archives → dist/ + checksums.txt
	mkdir -p dist
	go mod tidy
	@set -e; for platform in $(PLATFORMS); do \
		OS=$$(echo $$platform | cut -d/ -f1); \
		ARCH=$$(echo $$platform | cut -d/ -f2); \
		EXT=$$([ "$$OS" = "windows" ] && echo ".exe" || echo ""); \
		OUT=dist/$(NAME)_$${OS}_$${ARCH}$${EXT}; \
		echo "building $$OS/$$ARCH"; \
		GOOS=$$OS GOARCH=$$ARCH CGO_ENABLED=0 go build -trimpath \
			-ldflags "$(RELEASE_LDFLAGS)" -o $$OUT ./cmd/romctl; \
	done
	@echo "==> Checksums"
	cd dist && sha256sum * > checksums.txt && cat checksums.txt

clean: ## Remove build/ and dist/
	rm -rf build dist

help: ## Show this help message
	@awk 'BEGIN {FS = ":.*##"; printf "Usage: make \033[36m<target>\033[0m\n\nTargets:\n"} \
	     /^[a-zA-Z0-9_-]+:.*##/ {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

#-------------------------------------------------------------------------------
# romctl-specific: fetching and building the four native tools it still
# shells out to (chdman, dolphin-tool, extract-xiso, skyscraper), staged for
# embedding via go:embed. Unlike the fetch steps in some sibling projects,
# these are deliberately NOT dependencies of build/release — they pull and
# compile multi-GB C/C++ source trees (MAME, Dolphin), which is slow and
# optional: every other command works with nothing embedded, falling back
# to PATH (internal/tools).
#-------------------------------------------------------------------------------

# Fetched upstream C/C++ sources. Deliberately not named "vendor": Go's
# toolchain treats a top-level vendor/ directory specially (Go module
# vendoring), and that breaks `go build` for this module.
VENDOR  := thirdparty
DATADIR := internal/tools/data

MAME_URL         := https://github.com/mamedev/mame.git
MAME_REF         := mame0289
DOLPHIN_URL      := https://github.com/dolphin-emu/dolphin.git
DOLPHIN_REF      := 2609a
EXTRACT_XISO_URL := https://github.com/XboxDev/extract-xiso.git
EXTRACT_XISO_REF := v2.7.1
SKYSCRAPER_URL    := https://github.com/muldjord/skyscraper.git
SKYSCRAPER_REF    := 3.7.7

# Dolphin submodules dolphin-tool never uses (Windows/Android binaries, GUI,
# disabled features). Skipping them saves several hundred MB of fetching.
DOLPHIN_SKIP := Externals/Qt Externals/FFmpeg-bin Externals/wil Externals/libadrenotools \
	Externals/gtest Externals/mGBA/mgba Externals/SDL/SDL Externals/miniupnpc/miniupnp \
	Externals/rcheevos/rcheevos Externals/cubeb/cubeb

# MAME's project generator probes for the SDL OSD's optional backends; turn off
# everything chdman does not need. SDL2 headers are still required to configure.
MAME_FLAGS := TOOLS=1 EMULATOR=0 NOWERROR=1 USE_QTDEBUG=0 NO_X11=1 NO_USE_XINPUT=1 \
	NO_OPENGL=1 USE_WAYLAND=0 NO_USE_MIDI=1 NO_USE_PORTAUDIO=1 NO_USE_PULSEAUDIO=1 \
	NO_USE_PIPEWIRE=1 USE_SYSTEM_LIB_ZLIB=1

# MinGW-w64 cross toolchain prefix for tools-windows-amd64 (Fedora package
# names: mingw64-gcc-c++, mingw64-zlib, mingw64-sdl2-compat).
MINGW64_PREFIX := /usr/bin/x86_64-w64-mingw32-

JOBS   ?= $(shell nproc 2>/dev/null || echo 4)
CC     ?= gcc
# Fedora package: qt5-qtbase-devel. On Debian/Ubuntu it's qt5-qmake/qmake-qt5.
QMAKE  ?= qmake-qt5

# fetch_ref(dir, url, ref): shallow fetch of a tag, branch or commit.
define fetch_ref
	rm -rf $(1)
	git init -q $(1)
	git -C $(1) remote add origin $(2)
	git -C $(1) fetch -q --depth 1 origin $(3)
	git -C $(1) -c advice.detachedHead=false checkout -q FETCH_HEAD
endef

fetch-mame: $(VENDOR)/mame/.fetched
fetch-dolphin: $(VENDOR)/dolphin/.fetched
fetch-extract-xiso: $(VENDOR)/extract-xiso/.fetched
fetch-skyscraper: $(VENDOR)/skyscraper/.fetched

$(VENDOR)/mame/.fetched:
	$(call fetch_ref,$(@D),$(MAME_URL),$(MAME_REF))
	touch $@

$(VENDOR)/dolphin/.fetched:
	$(call fetch_ref,$(@D),$(DOLPHIN_URL),$(DOLPHIN_REF))
	git -C $(@D) submodule update -q --init --recursive --depth 1 -- . \
		$(addprefix ':!',$(DOLPHIN_SKIP))
	touch $@

# extract-xiso's upstream NO_FTP build mode is broken (stub macros that
# don't compile) and its bundled win32/dirent.c shim (written for old MSVC,
# which has no dirent.h) misbehaves under MinGW, which has a real one. We
# don't use FTP mode at all, so patches/extract-xiso-no-ftp.patch fixes the
# stubs and prefers MinGW's own dirent.h, dropping the libftp dependency
# entirely (it doesn't cross-compile for Windows cleanly either).
$(VENDOR)/extract-xiso/.fetched:
	$(call fetch_ref,$(@D),$(EXTRACT_XISO_URL),$(EXTRACT_XISO_REF))
	git -C $(@D) apply $(CURDIR)/patches/extract-xiso-no-ftp.patch
	touch $@

$(VENDOR)/skyscraper/.fetched:
	$(call fetch_ref,$(@D),$(SKYSCRAPER_URL),$(SKYSCRAPER_REF))
	touch $@

HOST_GOOS    := $(shell go env GOOS)
HOST_GOARCH  := $(shell go env GOARCH)
HOST_OS_DEFINE := __$(shell uname | tr a-z A-Z)__

tools: $(VENDOR)/mame/.fetched $(VENDOR)/dolphin/.fetched $(VENDOR)/extract-xiso/.fetched $(VENDOR)/skyscraper/.fetched ## Fetch & build chdman/dolphin-tool/extract-xiso/skyscraper for the host platform
	$(MAKE) -C $(VENDOR)/mame -j$(JOBS) $(MAME_FLAGS)
	mkdir -p $(DATADIR)/$(HOST_GOOS)_$(HOST_GOARCH)
	install -m 755 $(VENDOR)/mame/chdman $(DATADIR)/$(HOST_GOOS)_$(HOST_GOARCH)/chdman
	cmake -S $(VENDOR)/dolphin -B $(VENDOR)/dolphin/build -DCMAKE_BUILD_TYPE=Release \
		-DENABLE_QT=OFF -DENABLE_NOGUI=OFF -DENABLE_CLI_TOOL=ON -DENABLE_TESTS=OFF \
		-DENABLE_AUTOUPDATE=OFF -DENABLE_ANALYTICS=OFF -DUSE_DISCORD_PRESENCE=OFF \
		-DUSE_UPNP=OFF -DENABLE_LLVM=OFF -DENCODE_FRAMEDUMPS=OFF -DUSE_MGBA=OFF \
		-DENABLE_X11=OFF -DENABLE_EGL=OFF -DENABLE_SDL=OFF -DENABLE_EVDEV=OFF \
		-DENABLE_HWDB=OFF -DENABLE_ALSA=OFF -DENABLE_PULSEAUDIO=OFF -DENABLE_CUBEB=OFF \
		-DUSE_RETRO_ACHIEVEMENTS=OFF
	cmake --build $(VENDOR)/dolphin/build --target dolphin-tool -j$(JOBS)
	install -m 755 $(VENDOR)/dolphin/build/Binaries/dolphin-tool $(DATADIR)/$(HOST_GOOS)_$(HOST_GOARCH)/dolphin-tool
	cd $(VENDOR)/extract-xiso && $(CC) -O2 -DNO_FTP -D$(HOST_OS_DEFINE) -std=gnu89 -fcommon -o extract-xiso extract-xiso.c
	install -m 755 $(VENDOR)/extract-xiso/extract-xiso $(DATADIR)/$(HOST_GOOS)_$(HOST_GOARCH)/extract-xiso
	cd $(VENDOR)/skyscraper && $(QMAKE) skyscraper.pro && $(MAKE) -j$(JOBS)
	install -m 755 $(VENDOR)/skyscraper/Skyscraper $(DATADIR)/$(HOST_GOOS)_$(HOST_GOARCH)/skyscraper

tools-windows-amd64: $(VENDOR)/mame/.fetched $(VENDOR)/extract-xiso/.fetched ## Cross-build chdman/extract-xiso for Windows from Linux via MinGW-w64
	$(MAKE) -C $(VENDOR)/mame -j$(JOBS) TOOLS=1 EMULATOR=0 NOWERROR=1 USE_SYSTEM_LIB_ZLIB=1 \
		TARGETOS=windows PTR64=1 MINGW64=1 TOOLCHAIN=$(MINGW64_PREFIX)
	mkdir -p $(DATADIR)/windows_amd64
	install -m 755 $(VENDOR)/mame/chdman.exe $(DATADIR)/windows_amd64/chdman.exe
	cd $(VENDOR)/extract-xiso && $(MINGW64_PREFIX)gcc -O2 -DNO_FTP -std=gnu89 -fcommon \
		-o extract-xiso.exe extract-xiso.c -lws2_32
	install -m 755 $(VENDOR)/extract-xiso/extract-xiso.exe $(DATADIR)/windows_amd64/extract-xiso.exe

distclean: clean ## Remove build output AND fetched chdman/dolphin-tool/extract-xiso/skyscraper sources (several GB)
	rm -rf $(VENDOR)
