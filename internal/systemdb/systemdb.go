// Package systemdb holds the static tables of known ROM systems, their
// classes, and their BIOS requirements — a direct port of the SYSTEM_CLASS,
// BIOS_REQUIRED and BIOS_MD5 associative arrays from the Bash script.
package systemdb

// Class identifies how a system's games are stored and normalized.
type Class string

const (
	ClassCD      Class = "cd"      // CD-based; target format CHD (createcd).
	ClassDVD     Class = "dvd"     // DVD-based; target format CHD (createdvd, 2 KiB hunks).
	ClassUMD     Class = "umd"     // PSP; target format CHD (createdvd, 2 KiB hunks; PPSSPP 1.17+).
	ClassArcade  Class = "arcade"  // Set-named zips; never renamed, never unpacked.
	ClassDolphin Class = "dolphin" // GameCube/Wii; target format RVZ.
	ClassXbox    Class = "xbox"    // Original Xbox; target format XISO.
	ClassPS3     Class = "ps3"     // PS3; disc dumps as ISO (or Title.ps3/ folders).
	ClassN64     Class = "n64"     // N64; target format big-endian .z64.
	ClassSNES    Class = "snes"    // Headerless .sfc.
	ClassCart    Class = "cart"    // Other cartridge/handheld systems; bare files, no archives.
)

// SystemClass maps a system directory name to its Class.
var SystemClass = map[string]Class{
	"psx": ClassCD, "saturn": ClassCD, "saturnjp": ClassCD, "segacd": ClassCD,
	"megacd": ClassCD, "megacdjp": ClassCD, "pcenginecd": ClassCD, "tg-cd": ClassCD,
	"dreamcast": ClassCD, "3do": ClassCD, "neogeocd": ClassCD, "neogeocdjp": ClassCD,
	"pcfx": ClassCD, "amigacd32": ClassCD, "cdtv": ClassCD, "cdimono1": ClassCD,
	"atarijaguarcd": ClassCD,

	"ps2": ClassDVD,
	"psp": ClassUMD,

	"arcade": ClassArcade, "fbneo": ClassArcade, "fba": ClassArcade, "mame": ClassArcade,
	"mame-advmame": ClassArcade, "mame-mame4all": ClassArcade, "neogeo": ClassArcade,
	"cps": ClassArcade, "cps1": ClassArcade, "cps2": ClassArcade, "cps3": ClassArcade,
	"naomi": ClassArcade, "naomi2": ClassArcade, "naomigd": ClassArcade,
	"atomiswave": ClassArcade, "model2": ClassArcade, "model3": ClassArcade,
	"stv": ClassArcade, "daphne": ClassArcade, "consolearcade": ClassArcade,
	"triforce": ClassArcade,

	"gc": ClassDolphin, "wii": ClassDolphin,

	"xbox": ClassXbox,
	"ps3":  ClassPS3,
	"n64":  ClassN64,

	"snes": ClassSNES, "sfc": ClassSNES, "snesna": ClassSNES,

	"atari2600": ClassCart, "atari5200": ClassCart, "atari7800": ClassCart,
	"atarilynx": ClassCart, "atarijaguar": ClassCart, "nes": ClassCart,
	"famicom": ClassCart, "fds": ClassCart, "gb": ClassCart, "gbc": ClassCart,
	"gba": ClassCart, "sgb": ClassCart, "nds": ClassCart, "virtualboy": ClassCart,
	"pokemini": ClassCart, "gamegear": ClassCart, "mastersystem": ClassCart,
	"mark3": ClassCart, "genesis": ClassCart, "megadrive": ClassCart,
	"megadrivejp": ClassCart, "sega32x": ClassCart, "sega32xjp": ClassCart,
	"sega32xna": ClassCart, "sg-1000": ClassCart, "tg16": ClassCart,
	"pcengine": ClassCart, "supergrafx": ClassCart, "ngp": ClassCart,
	"ngpc": ClassCart, "wonderswan": ClassCart, "wonderswancolor": ClassCart,
	"colecovision": ClassCart, "intellivision": ClassCart, "vectrex": ClassCart,
	"odyssey2": ClassCart, "videopac": ClassCart, "channelf": ClassCart,
	"satellaview": ClassCart, "sufami": ClassCart, "megaduck": ClassCart,
	"supervision": ClassCart, "gamecom": ClassCart, "n64dd": ClassCart,
}

// BiosGroup is a set of alternative BIOS filenames (glob patterns allowed);
// at least one must be present. BiosRequired maps a system to the groups
// that must each be satisfied.
type BiosGroup = []string

// BiosRequired maps a system name to its required BIOS groups. Paths are
// relative to bios/. Every hash in BiosMD5 that covers a file named here
// (and every hash below it) has been cross-checked against the
// independently-maintained RetroDECK BIOS pack's own per-file manifest
// (community-sourced MD5/SHA1/size data, not guessed or hand-typed) - see
// BiosMD5's doc comment.
var BiosRequired = map[string][]BiosGroup{
	"psx":        {{"scph5501.bin", "scph5500.bin", "scph5502.bin"}},
	"ps2":        {{"ps2-*.bin"}},
	"saturn":     {{"mpr-17933.bin"}},
	"saturnjp":   {{"sega_101.bin"}},
	"segacd":     {{"bios_CD_U.bin", "bios_CD_E.bin", "bios_CD_J.bin"}},
	"megacd":     {{"bios_CD_E.bin", "bios_CD_U.bin", "bios_CD_J.bin"}},
	"megacdjp":   {{"bios_CD_J.bin"}},
	"pcenginecd": {{"syscard3.pce"}},
	"tg-cd":      {{"syscard3.pce"}},
	"dreamcast":  {{"dc/dc_boot.bin"}},
	"naomi":      {{"dc/naomi.zip"}},
	"atomiswave": {{"dc/awbios.zip"}},
	"3do":        {{"panafz10.bin"}},
	"atarilynx":  {{"lynxboot.img"}},
	"atari5200":  {{"5200.rom"}},
	"fds":        {{"disksys.rom"}},
	"xbox":       {{"mcpx_1.0.bin"}, {"Complex.bin"}, {"xbox_hdd.qcow2"}},
	"ps3":        {{"rpcs3/dev_flash/vsh/module"}},
}

// BiosOptional maps a system name to BIOS/firmware groups (same shape as
// BiosRequired - a group is one or more alternatives, any one satisfies
// it) that improve accuracy, compatibility or boot-logo behavior but
// aren't required for the system to run games at all - unlike
// BiosRequired, missing one of these is never a FAIL or WARN, only INFO
// (see commands.checkBios), regardless of whether the system has games.
var BiosOptional = map[string][]BiosGroup{
	"nds":        {{"bios7.bin"}, {"bios9.bin"}, {"firmware.bin"}},
	"gb":         {{"gb_bios.bin"}},
	"gbc":        {{"gbc_bios.bin"}},
	"gba":        {{"gba_bios.bin"}},
	"sgb":        {{"sgb_bios.bin"}},
	"pcenginecd": {{"syscard1.pce"}, {"syscard2.pce"}, {"gexpress.pce"}},
	"tg-cd":      {{"syscard1.pce"}, {"syscard2.pce"}, {"gexpress.pce"}},
	"atari7800":  {{"7800 BIOS (U).rom"}},
	// Dolphin's IPL.bin is per-region (one covers all games of that
	// region); HLE works fine without it for the vast majority of
	// GameCube/Wii games, so any one - or none - is fine.
	"gc": {{"GC/USA/IPL.bin", "GC/EUR/IPL.bin", "GC/JAP/IPL.bin"}},
}

// BiosMD5 holds reference MD5s for BIOS files with a single canonical
// dump - cross-checked against the independently-maintained RetroDECK
// BIOS pack's own per-file manifest (its generator hashes and records
// every file it ships, rather than this table being hand-typed from one
// source), though that alone isn't sufficient: a manifest only proves
// what RetroDECK's own pack happens to ship, not that every legitimately
// sourced copy elsewhere hashes the same way. Deliberately excluded even
// though the pack ships a copy:
//
//   - xbox_hdd.qcow2 (an Xbox's virtual hard disk holds game saves on a
//     real one, so a "canonical" hash would false-positive on every
//     genuine save) and NDS firmware.bin (holds a real console's Wi-Fi
//     MAC address and user settings, likewise never actually fixed) -
//     see commands.checkBios, which size-checks firmware.bin instead.
//   - dc/naomi.zip and dc/awbios.zip: these used to be here, but a zip's
//     own byte layout (compression, entry order, timestamps) varies
//     between tools/sources even when the ROM content inside is
//     byte-identical and correct - confirmed the hard way: a real user's
//     naomi.zip, hashing to a value this table rejected as "wrong",
//     turned out to match Batocera's own independently documented NAOMI
//     BIOS MD5 exactly. Hashing an archive's outer bytes was simply the
//     wrong check; presence is all that's actually verifiable here.
//   - Complex.bin (Xbox's "Complex" flash BIOS replacement): at least two
//     different builds circulate under this exact filename with
//     different hashes, cross-checked against the same real user's file
//   - it matched GameTechWiki's documented hash for Xbox kernel
//     version 1.00.4627.01 exactly, a different (and equally legitimate)
//     value than this table's former reference, which in turn differed
//     again from a third source's (Batocera's) hash for the same name.
//     No single MD5 is actually canonical here; presence is.
var BiosMD5 = map[string]string{
	"scph5500.bin":      "8dd7d5296a650fac7319bce665a6a53c",
	"scph5501.bin":      "490f666e1afb15b7362b406ed1cea246",
	"scph5502.bin":      "32736f17079d0b2b7024407c39bd3050",
	"mpr-17933.bin":     "3240872c70984b6cbfda1586cab68dbe",
	"sega_101.bin":      "85ec9ca47d8f6807718151cbcca8b964",
	"bios_CD_U.bin":     "2efd74e3232ff260e371b99f84024f7f",
	"bios_CD_E.bin":     "e66fa1dc5820d254611fdcdba0662372",
	"bios_CD_J.bin":     "278a9397d192149e84e820ac621a8edd",
	"syscard1.pce":      "2b7ccb3d86baa18f6402c176f3065082",
	"syscard2.pce":      "3cdd6614a918616bfc41c862e889dd79",
	"syscard3.pce":      "38179df8f4ac870017db21ebcbf53114",
	"gexpress.pce":      "6d2cb14fc3e1f65ceb135633d1694122",
	"dc/dc_boot.bin":    "e10c53c2f8b90bab96ead2d368858623",
	"panafz10.bin":      "51f2f43ae2f3508a14d9f56597e2d3ce",
	"lynxboot.img":      "fcd403db69f54290b51035d82f835e7b",
	"5200.rom":          "281f20ea4320404ec820fb7ec0693b38",
	"7800 BIOS (U).rom": "0763f1ffb006ddbe32e52d497ee848ae",
	"disksys.rom":       "ca30b50f880eb660a320674ed365ef7a",
	"gb_bios.bin":       "32fbbd84168d3482956eb3c5051637f5",
	"gbc_bios.bin":      "dbfce9db9deaa2567f6a84fde55f9680",
	"sgb_bios.bin":      "d574d4f9c12f305074798f54c091a8b4",
	"gba_bios.bin":      "a860e8c0b6d573d191e4ec7db1b1e4f6",
	"bios7.bin":         "df692a80a5b1bc90728bc3dfc76cd948",
	"bios9.bin":         "a392174eb3e572fed6447e956bde4b25",
	// mcpx_1.0.bin: confirmed, not cut - a fixed silicon boot ROM burned
	// into the MCPX chip, identical on every stock Xbox of this revision.
	// Both xemu's own documentation and Batocera's wiki independently
	// confirm this exact hash.
	"mcpx_1.0.bin":   "d49c52a4102f6df7bcf8d0617ac475ed",
	"GC/USA/IPL.bin": "b17148254a5799684c7d783206504926",
	"GC/EUR/IPL.bin": "db92574caab77a7ec99d4605fd6f2450",
	"GC/JAP/IPL.bin": "fc924a7c879b661abc37cec4f018fdf3",
}

// BiosSize holds an expected file size, in bytes, for a BIOS/firmware
// file with no single canonical hash (a real console's copy legitimately
// varies - see BiosMD5's doc comment) but a fixed, well-known size that's
// still worth checking: catches a truncated or wrong-file placeholder.
var BiosSize = map[string]int64{
	"firmware.bin": 262144,
}

// BiosLabel gives one short, human phrase for a BiosRequired/BiosOptional
// entry, keyed the same way Fixes/the bios inventory look it up: by
// system for a BiosRequired group (several alternatives, one phrase), or
// "system/filename" for a single BiosOptional file (several unrelated
// optional files can share a system). Falls back to a generic phrase
// (see commands' biosLabel) when an entry isn't listed here, so a future
// addition to BiosRequired/BiosOptional never goes unlabeled.
var BiosLabel = map[string]string{
	"psx":                         "PS1 BIOS (any NTSC-U/NTSC-J/PAL dump)",
	"ps2":                         "PS2 BIOS (console-specific; many valid revisions, no single hash)",
	"saturn":                      "Saturn BIOS (US/EU v1.01a)",
	"saturnjp":                    "Saturn BIOS (JP)",
	"segacd":                      "Sega CD BIOS (US/EU/JP)",
	"megacd":                      "Mega CD BIOS (EU/US/JP)",
	"megacdjp":                    "Mega CD BIOS (JP)",
	"pcenginecd":                  "PC Engine CD System Card 3.0",
	"tg-cd":                       "TurboGrafx-CD System Card 3.0",
	"dreamcast":                   "Dreamcast boot ROM",
	"naomi":                       "Naomi BIOS",
	"atomiswave":                  "Atomiswave BIOS",
	"3do":                         "3DO BIOS (Panasonic FZ-10)",
	"atarilynx":                   "Atari Lynx boot ROM",
	"atari5200":                   "Atari 5200 BIOS",
	"fds":                         "Famicom Disk System BIOS",
	"xbox/mcpx_1.0.bin":           "Xbox MCPX boot ROM",
	"xbox/Complex.bin":            "Xbox kernel (Complex.bin)",
	"xbox/xbox_hdd.qcow2":         "Xbox virtual hard disk (created on first boot; holds saves, not firmware)",
	"ps3":                         "PS3 system firmware (install via RPCS3 from PS3UPDAT.PUP, not one file)",
	"nds/bios7.bin":               "DS ARM7 BIOS (melonDS has a free replacement)",
	"nds/bios9.bin":               "DS ARM9 BIOS (melonDS has a free replacement)",
	"nds/firmware.bin":            "DS firmware (per-console MAC address; size-checked, not hashed)",
	"gb/gb_bios.bin":              "Game Boy boot ROM (boot logo only)",
	"gbc/gbc_bios.bin":            "Game Boy Color boot ROM (boot logo only)",
	"gba/gba_bios.bin":            "Game Boy Advance boot ROM (a few accuracy edge cases only)",
	"sgb/sgb_bios.bin":            "Super Game Boy boot ROM",
	"pcenginecd/syscard1.pce":     "PC Engine CD System Card 1.0 (superseded by 3.0; a few early titles only)",
	"pcenginecd/syscard2.pce":     "PC Engine CD System Card 2.0 (superseded by 3.0; a few early titles only)",
	"pcenginecd/gexpress.pce":     "PC Engine Games Express card (a handful of specific titles only)",
	"tg-cd/syscard1.pce":          "TurboGrafx-CD System Card 1.0 (superseded by 3.0; a few early titles only)",
	"tg-cd/syscard2.pce":          "TurboGrafx-CD System Card 2.0 (superseded by 3.0; a few early titles only)",
	"tg-cd/gexpress.pce":          "TurboGrafx Express card (a handful of specific titles only)",
	"atari7800/7800 BIOS (U).rom": "Atari 7800 BIOS (a handful of specific titles only)",
	"gc/GC/USA/IPL.bin":           "GameCube/Wii IPL boot ROM (per-region; HLE works fine without it for almost every game)",
}

// ClassOf returns the known Class for a system, or "" if unknown.
func ClassOf(system string) Class {
	return SystemClass[system]
}

// Names returns every known system name.
func Names() []string {
	names := make([]string, 0, len(SystemClass))
	for s := range SystemClass {
		names = append(names, s)
	}
	return names
}
