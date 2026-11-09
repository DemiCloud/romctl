package systemdb

// DatSource names, for one system, the No-Intro/Redump DAT that lists its
// known games and their CRC32/MD5/SHA1 hashes (see internal/datdb). System
// is that provider's own pack name, not romctl's directory name - the
// exact string its DAT file (and the folder it ships in) is named upstream.
type DatSource struct {
	Provider string // "no-intro" (cartridge/handheld) or "redump" (disc-based)
	System   string
}

// Dat maps a system directory name to its DatSource, same convention as
// SkyscraperPlatform: built by class (ClassCD/ClassDVD/ClassUMD/ClassXbox/
// ClassDolphin/ClassPS3 -> redump, everything else cartridge-shaped ->
// no-intro), with the provider's actual pack name spelled out per entry
// rather than derived, since No-Intro/Redump naming doesn't follow a fixed
// pattern from romctl's own system names. A system with no real DAT
// coverage (arcade - MAME has its own, structurally different, ROM-set
// metadata, not a hash-per-dump list; a few laserdisc/hybrid niches with no
// dedicated pack) is left out rather than guessed.
//
// These strings are believed correct as of this writing but are the one
// part of this table `make dats` actually re-verifies: it checks a staged
// DAT's own <header><name> against the System value here before embedding
// it, so a drifted or renamed upstream pack name is caught there, not
// silently mismatched at runtime.
var Dat = map[string]DatSource{
	// Redump (disc-based)
	"psx":           {"redump", "Sony - PlayStation"},
	"saturn":        {"redump", "Sega - Saturn"},
	"saturnjp":      {"redump", "Sega - Saturn"},
	"segacd":        {"redump", "Sega - Mega-CD - Sega CD"},
	"megacd":        {"redump", "Sega - Mega-CD - Sega CD"},
	"megacdjp":      {"redump", "Sega - Mega-CD - Sega CD"},
	"pcenginecd":    {"redump", "NEC - PC Engine CD & TurboGrafx CD"},
	"tg-cd":         {"redump", "NEC - PC Engine CD & TurboGrafx CD"},
	"dreamcast":     {"redump", "Sega - Dreamcast"},
	"3do":           {"redump", "Panasonic - 3DO Interactive Multiplayer"},
	"neogeocd":      {"redump", "SNK - NeoGeo CD"},
	"neogeocdjp":    {"redump", "SNK - NeoGeo CD"},
	"pcfx":          {"redump", "NEC - PC-FX & PC-FXGA"},
	"amigacd32":     {"redump", "Commodore - Amiga CD32"},
	"cdtv":          {"redump", "Commodore - Amiga CDTV"},
	"cdimono1":      {"redump", "Philips - CD-i"},
	"atarijaguarcd": {"redump", "Atari - Jaguar CD Interactive Multiplayer"},
	"ps2":           {"redump", "Sony - PlayStation 2"},
	"psp":           {"redump", "Sony - PlayStation Portable"},
	"gc":            {"redump", "Nintendo - GameCube"},
	"wii":           {"redump", "Nintendo - Wii"},
	"xbox":          {"redump", "Microsoft - Xbox"},
	"ps3":           {"redump", "Sony - PlayStation 3"},

	// No-Intro (cartridge/handheld)
	"n64":   {"no-intro", "Nintendo - Nintendo 64"},
	"n64dd": {"no-intro", "Nintendo - Nintendo 64DD"},

	"snes": {"no-intro", "Nintendo - Super Nintendo Entertainment System"},
	"sfc":  {"no-intro", "Nintendo - Super Nintendo Entertainment System"},
	// US-region SNES carts are the same underlying DAT as snes/sfc; no
	// dedicated per-region pack exists.
	"snesna": {"no-intro", "Nintendo - Super Nintendo Entertainment System"},

	"atari2600":   {"no-intro", "Atari - 2600"},
	"atari5200":   {"no-intro", "Atari - 5200"},
	"atari7800":   {"no-intro", "Atari - 7800"},
	"atarilynx":   {"no-intro", "Atari - Lynx"},
	"atarijaguar": {"no-intro", "Atari - Jaguar"},

	"nes": {"no-intro", "Nintendo - Nintendo Entertainment System"},
	// Famicom carts are the same pack as nes - No-Intro doesn't split NES
	// and Famicom into separate DATs.
	"famicom":     {"no-intro", "Nintendo - Nintendo Entertainment System"},
	"fds":         {"no-intro", "Nintendo - Family Computer Disk System"},
	"gb":          {"no-intro", "Nintendo - Game Boy"},
	"gbc":         {"no-intro", "Nintendo - Game Boy Color"},
	"gba":         {"no-intro", "Nintendo - Game Boy Advance"},
	"sgb":         {"no-intro", "Nintendo - Game Boy"}, // Super Game Boy runs plain Game Boy carts
	"nds":         {"no-intro", "Nintendo - Nintendo DS"},
	"virtualboy":  {"no-intro", "Nintendo - Virtual Boy"},
	"pokemini":    {"no-intro", "Nintendo - Pokemon Mini"},
	"satellaview": {"no-intro", "Nintendo - Satellaview"},
	"sufami":      {"no-intro", "Nintendo - Sufami Turbo"},

	"gamegear":     {"no-intro", "Sega - Game Gear"},
	"mastersystem": {"no-intro", "Sega - Master System - Mark III"},
	"mark3":        {"no-intro", "Sega - Master System - Mark III"},
	"genesis":      {"no-intro", "Sega - Mega Drive - Genesis"},
	"megadrive":    {"no-intro", "Sega - Mega Drive - Genesis"},
	"megadrivejp":  {"no-intro", "Sega - Mega Drive - Genesis"},
	"sega32x":      {"no-intro", "Sega - 32X"},
	"sega32xjp":    {"no-intro", "Sega - 32X"},
	"sega32xna":    {"no-intro", "Sega - 32X"},
	"sg-1000":      {"no-intro", "Sega - SG-1000"},

	"tg16":     {"no-intro", "NEC - PC Engine - TurboGrafx-16"},
	"pcengine": {"no-intro", "NEC - PC Engine - TurboGrafx-16"},
	// SuperGrafx carts are cataloged inside the main PC Engine set; no
	// dedicated SuperGrafx pack exists.
	"supergrafx": {"no-intro", "NEC - PC Engine - TurboGrafx-16"},

	"ngp":             {"no-intro", "SNK - NeoGeo Pocket"},
	"ngpc":            {"no-intro", "SNK - NeoGeo Pocket Color"},
	"wonderswan":      {"no-intro", "Bandai - WonderSwan"},
	"wonderswancolor": {"no-intro", "Bandai - WonderSwan Color"},
	"colecovision":    {"no-intro", "Coleco - ColecoVision"},
	"intellivision":   {"no-intro", "Mattel - Intellivision"},
	"vectrex":         {"no-intro", "GCE - 4-Switch Vectrex"},
	"odyssey2":        {"no-intro", "Magnavox - Odyssey2 - Videopac+"},
	"videopac":        {"no-intro", "Magnavox - Odyssey2 - Videopac+"},
	"channelf":        {"no-intro", "Fairchild - Channel F"},
	"megaduck":        {"no-intro", "Creatronic - Mega Duck - Cougar Boy"},
	"supervision":     {"no-intro", "Watara - Supervision"},
	"gamecom":         {"no-intro", "Tiger - Game.com"},
}
