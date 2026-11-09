package systemdb

// DatSource names, for one system, the No-Intro/Redump DAT that lists its
// known games and their CRC32/MD5/SHA1 hashes (see internal/datdb). System
// is that provider's own pack name - exactly matching that DAT's own
// <header><name> - including a format-variant qualifier when the provider
// publishes more than one DAT for the same system (e.g. "(BigEndian)" vs
// "(ByteSwapped)" for N64, "(Headered)" vs "(Headerless)" for NES); the
// variant chosen here matches romctl's own target format where one exists
// (see Class's doc comments), otherwise a best-effort pick - see the
// inline notes below for the genuinely uncertain ones.
//
// Real No-Intro/Redump filenames also carry their own dynamic
// "<System> (<timestamp>).dat" suffix that changes release to release;
// System here is deliberately just the stable part. internal/datdb's
// Stage function (driven by `make dats`) matches a downloaded file to
// its System entry by glob (ignoring the timestamp) and strips it when
// staging for embedding, so Load's own lookup can stay a plain exact
// match.
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
// The no-intro entries below were cross-checked against a real 2026-11-08
// "No-Intro Love Pack (DAT)" listing; the redump entries are still
// unverified against a real Redump download as of this writing (see
// AGENTS.md's "DAT snapshot status"). internal/datdb's Stage re-verifies
// a staged file's own <header><name> against the System value here
// before embedding it regardless, so a drifted or wrong pack name is
// caught there, not silently mismatched at runtime.
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
	//
	// romctl normalizes N64 ROMs to big-endian .z64 (ClassN64) - matches
	// the "(BigEndian)" DAT, not "(ByteSwapped)".
	"n64":   {"no-intro", "Nintendo - Nintendo 64 (BigEndian)"},
	"n64dd": {"no-intro", "Nintendo - Nintendo 64DD"},

	"snes": {"no-intro", "Nintendo - Super Nintendo Entertainment System"},
	"sfc":  {"no-intro", "Nintendo - Super Nintendo Entertainment System"},
	// US-region SNES carts are the same underlying DAT as snes/sfc; no
	// dedicated per-region pack exists.
	"snesna": {"no-intro", "Nintendo - Super Nintendo Entertainment System"},

	"atari2600": {"no-intro", "Atari - Atari 2600"},
	"atari5200": {"no-intro", "Atari - Atari 5200"},
	// README's "Out of scope" section already documents that romctl
	// expects headered No-Intro .a78 dumps for 7800, not ProSystem's
	// headerless convention - matches the "(A78)" DAT, not "(BIN)".
	"atari7800": {"no-intro", "Atari - Atari 7800 (A78)"},
	// Best-effort pick, unconfirmed: No-Intro publishes 3 Lynx container
	// variants (BLL/LNX/LYX) and 5 Jaguar ones (ABS/COF/J64/JAG/ROM) with
	// no romctl convention to match against. ".lnx"/".jag" are the most
	// common extensions in general emulator use; verify against your own
	// library's actual file extensions and correct these two if wrong.
	"atarilynx":   {"no-intro", "Atari - Atari Lynx (LNX)"},
	"atarijaguar": {"no-intro", "Atari - Atari Jaguar (JAG)"},

	// Headered is the standard iNES convention nearly every NES emulator
	// expects (unlike SNES's copier header, which is junk to strip).
	"nes": {"no-intro", "Nintendo - Nintendo Entertainment System (Headered)"},
	// Famicom carts are the same pack as nes - No-Intro doesn't split NES
	// and Famicom into separate DATs.
	"famicom": {"no-intro", "Nintendo - Nintendo Entertainment System (Headered)"},
	"fds":     {"no-intro", "Nintendo - Family Computer Disk System (FDS)"},
	"gb":      {"no-intro", "Nintendo - Game Boy"},
	"gbc":     {"no-intro", "Nintendo - Game Boy Color"},
	"gba":     {"no-intro", "Nintendo - Game Boy Advance"},
	"sgb":     {"no-intro", "Nintendo - Game Boy"}, // Super Game Boy runs plain Game Boy carts
	// Decrypted matches a flashcart dump played directly in an emulator;
	// "(Encrypted)" is the raw cart-as-dumped form most users won't have.
	"nds":         {"no-intro", "Nintendo - Nintendo DS (Decrypted)"},
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
	"sg-1000":      {"no-intro", "Sega - SG-1000 - SC-3000"},

	"tg16":     {"no-intro", "NEC - PC Engine - TurboGrafx-16"},
	"pcengine": {"no-intro", "NEC - PC Engine - TurboGrafx-16"},
	// SuperGrafx has its own dedicated DAT, separate from the main PC
	// Engine set.
	"supergrafx": {"no-intro", "NEC - PC Engine SuperGrafx"},

	"ngp":             {"no-intro", "SNK - NeoGeo Pocket"},
	"ngpc":            {"no-intro", "SNK - NeoGeo Pocket Color"},
	"wonderswan":      {"no-intro", "Bandai - WonderSwan"},
	"wonderswancolor": {"no-intro", "Bandai - WonderSwan Color"},
	"colecovision":    {"no-intro", "Coleco - ColecoVision"},
	"intellivision":   {"no-intro", "Mattel - Intellivision"},
	"vectrex":         {"no-intro", "GCE - Vectrex"},
	"odyssey2":        {"no-intro", "Magnavox - Odyssey 2"},
	"videopac":        {"no-intro", "Magnavox - Odyssey 2"},
	"channelf":        {"no-intro", "Fairchild - Channel F"},
	// Not Creatronic - Welback is the actual No-Intro publisher name for
	// this system.
	"megaduck":    {"no-intro", "Welback - Mega Duck"},
	"supervision": {"no-intro", "Watara - Supervision"},
	"gamecom":     {"no-intro", "Tiger - Game.com"},
}
