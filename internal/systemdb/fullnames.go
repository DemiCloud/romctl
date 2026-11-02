package systemdb

// FullName maps a system directory name to the human-readable name ES-DE
// shows for it, for display purposes (e.g. "list"). Systems with no entry
// here fall back to their directory name in DisplayName.
var FullName = map[string]string{
	"psx": "Sony PlayStation", "saturn": "Sega Saturn", "saturnjp": "Sega Saturn (Japan)",
	"segacd": "Sega CD", "megacd": "Sega Mega-CD", "megacdjp": "Sega Mega-CD (Japan)",
	"pcenginecd": "NEC PC Engine CD", "tg-cd": "NEC TurboGrafx-CD", "dreamcast": "Sega Dreamcast",
	"3do": "3DO Interactive Multiplayer", "neogeocd": "SNK Neo Geo CD",
	"neogeocdjp": "SNK Neo Geo CD (Japan)", "pcfx": "NEC PC-FX",
	"amigacd32": "Commodore Amiga CD32", "cdtv": "Commodore CDTV", "cdimono1": "Philips CD-i",
	"atarijaguarcd": "Atari Jaguar CD",

	"ps2": "Sony PlayStation 2",
	"psp": "Sony PlayStation Portable",

	"arcade": "Arcade", "fbneo": "Arcade (FinalBurn Neo)", "fba": "Arcade (FB Alpha)",
	"mame": "Arcade (MAME)", "mame-advmame": "Arcade (AdvanceMAME)",
	"mame-mame4all": "Arcade (MAME4All)", "neogeo": "SNK Neo Geo", "cps": "Capcom Play System",
	"cps1": "Capcom Play System I", "cps2": "Capcom Play System II", "cps3": "Capcom Play System III",
	"naomi": "Sega NAOMI", "naomi2": "Sega NAOMI 2", "naomigd": "Sega NAOMI GD-ROM",
	"atomiswave": "Sammy Atomiswave", "model2": "Sega Model 2", "model3": "Sega Model 3",
	"stv": "Sega ST-V", "daphne": "Arcade (LaserDisc)", "consolearcade": "Console Arcade",
	"triforce": "Namco/Sega/Nintendo Triforce",

	"gc": "Nintendo GameCube", "wii": "Nintendo Wii",

	"xbox": "Microsoft Xbox",
	"ps3":  "Sony PlayStation 3",
	"n64":  "Nintendo 64",

	"snes": "Super Nintendo Entertainment System", "sfc": "Super Famicom",
	"snesna": "Super Nintendo Entertainment System",

	"atari2600": "Atari 2600", "atari5200": "Atari 5200", "atari7800": "Atari 7800",
	"atarilynx": "Atari Lynx", "atarijaguar": "Atari Jaguar",
	"nes": "Nintendo Entertainment System", "famicom": "Nintendo Famicom",
	"fds": "Nintendo Famicom Disk System", "gb": "Nintendo Game Boy",
	"gbc": "Nintendo Game Boy Color", "gba": "Nintendo Game Boy Advance",
	"sgb": "Super Game Boy", "nds": "Nintendo DS", "virtualboy": "Nintendo Virtual Boy",
	"pokemini": "Nintendo Pokémon Mini", "gamegear": "Sega Game Gear",
	"mastersystem": "Sega Master System", "mark3": "Sega Mark III",
	"genesis": "Sega Genesis", "megadrive": "Sega Mega Drive",
	"megadrivejp": "Sega Mega Drive (Japan)", "sega32x": "Sega 32X",
	"sega32xjp": "Sega 32X (Japan)", "sega32xna": "Sega 32X (NA)", "sg-1000": "Sega SG-1000",
	"tg16": "NEC TurboGrafx-16", "pcengine": "NEC PC Engine", "supergrafx": "NEC PC Engine SuperGrafx",
	"ngp": "SNK Neo Geo Pocket", "ngpc": "SNK Neo Geo Pocket Color",
	"wonderswan": "Bandai WonderSwan", "wonderswancolor": "Bandai WonderSwan Color",
	"colecovision": "ColecoVision", "intellivision": "Mattel Intellivision",
	"vectrex": "GCE Vectrex", "odyssey2": "Magnavox Odyssey2", "videopac": "Philips Videopac",
	"channelf": "Fairchild Channel F", "satellaview": "Nintendo Satellaview",
	"sufami": "Bandai SuFami Turbo", "megaduck": "Creatronic Mega Duck",
	"supervision": "Watara Supervision", "gamecom": "Tiger Game.com", "n64dd": "Nintendo 64DD",
}

// DisplayName returns the human-readable name for system, falling back to
// the directory name itself when it isn't a known system.
func DisplayName(system string) string {
	if name, ok := FullName[system]; ok {
		return name
	}
	return system
}
