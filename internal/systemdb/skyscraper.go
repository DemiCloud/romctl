package systemdb

// SkyscraperPlatform maps a system directory name to the platform short
// name Skyscraper (github.com/muldjord/skyscraper) uses for its '-p' flag,
// built from its actual '--help' output (its own supported-platform list
// has nothing specific to many niche/regional systems; those fall back to
// the closest real platform Skyscraper does support, noted inline). A
// system with no entry here has no Skyscraper equivalent at all (original
// Xbox and PS3 aren't in Skyscraper's platform list; romctl scrape skips
// them, see internal/commands's Scrape) and is left out rather than
// guessed.
var SkyscraperPlatform = map[string]string{
	"psx": "psx", "saturn": "saturn", "saturnjp": "saturn",
	"segacd": "segacd", "megacd": "megacd", "megacdjp": "megacd",
	"pcenginecd": "pcenginecd", "tg-cd": "pcenginecd", "dreamcast": "dreamcast",
	"3do": "3do", "neogeocd": "neogeocd", "neogeocdjp": "neogeocd",
	"pcfx": "pcfx", "amigacd32": "amigacd32",
	"cdtv":          "amiga", // Commodore CDTV is Amiga-based; no dedicated Skyscraper platform
	"atarijaguarcd": "atarijaguarcd",

	"ps2": "ps2",
	"psp": "psp",

	"arcade": "arcade", "fbneo": "fba", "fba": "fba",
	"mame": "mame-libretro", "mame-advmame": "mame-advmame", "mame-mame4all": "mame-mame4all",
	"neogeo": "neogeo",
	"cps":    "arcade", "cps1": "arcade", "cps2": "arcade", "cps3": "arcade", // no dedicated CPS platform
	"naomi": "naomi", "naomi2": "naomi", "naomigd": "naomi",
	"atomiswave": "atomiswave",
	"stv":        "arcade", // Sega ST-V has no dedicated Skyscraper platform

	"gc": "gc", "wii": "wii",

	"n64": "n64",

	"snes": "snes", "sfc": "snes", "snesna": "snes",
	"satellaview": "snes", "sufami": "snes", // SNES peripherals/formats, no dedicated platform

	"atari2600": "atari2600", "atari5200": "atari5200", "atari7800": "atari7800",
	"atarilynx": "atarilynx", "atarijaguar": "atarijaguar",
	"nes": "nes", "famicom": "nes", "fds": "fds",
	"gb": "gb", "gbc": "gbc", "gba": "gba", "sgb": "gb",
	"nds": "nds", "virtualboy": "virtualboy", "pokemini": "pokemini",
	"gamegear": "gamegear", "mastersystem": "mastersystem", "mark3": "mastersystem",
	"genesis": "genesis", "megadrive": "megadrive", "megadrivejp": "megadrive",
	"sega32x": "sega32x", "sega32xjp": "sega32x", "sega32xna": "sega32x",
	"sg-1000": "sg-1000", "tg16": "pcengine", "pcengine": "pcengine",
	"supergrafx": "pcengine", // no dedicated SuperGrafx platform
	"ngp":        "ngp", "ngpc": "ngpc",
	"wonderswan": "wonderswan", "wonderswancolor": "wonderswancolor",
	"colecovision": "coleco", "intellivision": "intellivision", "vectrex": "vectrex",
	"odyssey2": "videopac", "videopac": "videopac", "channelf": "channelf",
	"n64dd": "n64", // 64DD, no dedicated platform
}
