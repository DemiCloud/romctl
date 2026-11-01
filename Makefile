# Build the external tools romctl uses from pinned upstream sources.
#
#   make            fetch and build everything into ./bin
#   make chdman     build one tool (chdman, dolphin-tool, maxcso, makeps3iso, 7z)
#   make lint       shellcheck + shfmt check of romctl
#   make clean      remove build output (keeps fetched sources)
#   make distclean  remove sources and build output
#
# romctl prefers ./bin over $PATH, so a tool built here wins over any system copy.

SHELL := /bin/bash
.DELETE_ON_ERROR:
.SUFFIXES:

JOBS   ?= $(shell nproc 2>/dev/null || echo 4)
VENDOR := vendor
BIN    := bin

# Pinned upstream versions. Bump these to update a tool.
MAME_URL     := https://github.com/mamedev/mame.git
MAME_REF     := mame0289
DOLPHIN_URL  := https://github.com/dolphin-emu/dolphin.git
DOLPHIN_REF  := 2609a
MAXCSO_URL   := https://github.com/unknownbrackets/maxcso.git
MAXCSO_REF   := v1.13.0
PS3ISO_URL   := https://github.com/bucanero/ps3iso-utils.git
PS3ISO_REF   := 878090980a9042c61901920fed1b034af215e8c7
SEVENZIP_URL := https://github.com/ip7z/7zip.git
SEVENZIP_REF := 26.05

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

TOOLS := $(BIN)/chdman $(BIN)/dolphin-tool $(BIN)/maxcso $(BIN)/makeps3iso $(BIN)/7z

.PHONY: all chdman dolphin-tool maxcso makeps3iso 7z fetch lint clean distclean

all: $(TOOLS)

chdman: $(BIN)/chdman
dolphin-tool: $(BIN)/dolphin-tool
maxcso: $(BIN)/maxcso
makeps3iso: $(BIN)/makeps3iso
7z: $(BIN)/7z

fetch: $(VENDOR)/mame/.fetched $(VENDOR)/dolphin/.fetched $(VENDOR)/maxcso/.fetched \
	$(VENDOR)/ps3iso-utils/.fetched $(VENDOR)/7zip/.fetched

# fetch_ref(dir, url, ref): shallow fetch of a tag, branch or commit.
define fetch_ref
	rm -rf $(1)
	git init -q $(1)
	git -C $(1) remote add origin $(2)
	git -C $(1) fetch -q --depth 1 origin $(3)
	git -C $(1) -c advice.detachedHead=false checkout -q FETCH_HEAD
endef

$(VENDOR)/mame/.fetched:
	$(call fetch_ref,$(@D),$(MAME_URL),$(MAME_REF))
	touch $@

$(VENDOR)/dolphin/.fetched:
	$(call fetch_ref,$(@D),$(DOLPHIN_URL),$(DOLPHIN_REF))
	git -C $(@D) submodule update -q --init --recursive --depth 1 -- . \
		$(addprefix ':!',$(DOLPHIN_SKIP))
	touch $@

$(VENDOR)/maxcso/.fetched:
	$(call fetch_ref,$(@D),$(MAXCSO_URL),$(MAXCSO_REF))
	git -C $(@D) submodule update -q --init --recursive --depth 1
	touch $@

$(VENDOR)/ps3iso-utils/.fetched:
	$(call fetch_ref,$(@D),$(PS3ISO_URL),$(PS3ISO_REF))
	touch $@

$(VENDOR)/7zip/.fetched:
	$(call fetch_ref,$(@D),$(SEVENZIP_URL),$(SEVENZIP_REF))
	touch $@

$(BIN):
	mkdir -p $@

# MAME: tools only, no emulator.
$(BIN)/chdman: $(VENDOR)/mame/.fetched | $(BIN)
	$(MAKE) -C $(VENDOR)/mame -j$(JOBS) $(MAME_FLAGS)
	install -m 755 $(VENDOR)/mame/chdman $@

# Dolphin: only the dolphin-tool target, no GUI or optional integrations.
$(BIN)/dolphin-tool: $(VENDOR)/dolphin/.fetched | $(BIN)
	cmake -S $(VENDOR)/dolphin -B $(VENDOR)/dolphin/build -DCMAKE_BUILD_TYPE=Release \
		-DENABLE_QT=OFF -DENABLE_NOGUI=OFF -DENABLE_CLI_TOOL=ON -DENABLE_TESTS=OFF \
		-DENABLE_AUTOUPDATE=OFF -DENABLE_ANALYTICS=OFF -DUSE_DISCORD_PRESENCE=OFF \
		-DUSE_UPNP=OFF -DENABLE_LLVM=OFF -DENCODE_FRAMEDUMPS=OFF -DUSE_MGBA=OFF \
		-DENABLE_X11=OFF -DENABLE_EGL=OFF -DENABLE_SDL=OFF -DENABLE_EVDEV=OFF \
		-DENABLE_HWDB=OFF -DENABLE_ALSA=OFF -DENABLE_PULSEAUDIO=OFF -DENABLE_CUBEB=OFF \
		-DUSE_RETRO_ACHIEVEMENTS=OFF
	cmake --build $(VENDOR)/dolphin/build --target dolphin-tool -j$(JOBS)
	install -m 755 $(VENDOR)/dolphin/build/Binaries/dolphin-tool $@

$(BIN)/maxcso: $(VENDOR)/maxcso/.fetched | $(BIN)
	$(MAKE) -C $(VENDOR)/maxcso -j$(JOBS)
	install -m 755 $(VENDOR)/maxcso/maxcso $@

$(BIN)/makeps3iso: $(VENDOR)/ps3iso-utils/.fetched | $(BIN)
	$(MAKE) -C $(VENDOR)/ps3iso-utils/makeps3iso
	install -m 755 $(VENDOR)/ps3iso-utils/makeps3iso/makeps3iso $@

# 7-Zip: the full-featured standalone 7zz, installed as 7z.
$(BIN)/7z: $(VENDOR)/7zip/.fetched | $(BIN)
	$(MAKE) -C $(VENDOR)/7zip/CPP/7zip/Bundles/Alone2 -f makefile.gcc -j$(JOBS)
	install -m 755 $(VENDOR)/7zip/CPP/7zip/Bundles/Alone2/_o/7zz $@

lint:
	shellcheck romctl
	shfmt -i 2 -ci -d romctl

clean:
	rm -rf $(BIN) $(VENDOR)/dolphin/build
	-$(MAKE) -C $(VENDOR)/mame clean >/dev/null 2>&1
	-$(MAKE) -C $(VENDOR)/maxcso clean >/dev/null 2>&1
	-$(MAKE) -C $(VENDOR)/ps3iso-utils/makeps3iso clean >/dev/null 2>&1
	rm -rf $(VENDOR)/7zip/CPP/7zip/Bundles/Alone2/_o

distclean:
	rm -rf $(BIN) $(VENDOR)
