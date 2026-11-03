package actions

import (
	"fmt"
	"regexp"
	"strings"
)

// Scheme selects how ApplyScheme treats region/dump tags in a ROM's name.
type Scheme string

const (
	// SchemeNoTag strips tags down to one definitive name per game (romctl's
	// original behavior): "Sonic CD", not "Sonic CD (USA) [!]".
	SchemeNoTag Scheme = "notag"
	// SchemeKeep leaves names exactly as found; ApplyScheme is a no-op.
	SchemeKeep Scheme = "keep"
	// SchemeNoIntro and SchemeRedump reorder whatever Region/Disc N/Rev N
	// tags are already present into the matching scene convention's own
	// order (confirmed against wiki.no-intro.org and Redump's forum - see
	// README.md's Naming section). Nothing else - languages, Beta/Proto/
	// Unl flags, bad-dump markers - is recognized: romctl only has the
	// existing filename to go on, no DAT data (out of scope, see
	// AGENTS.md), so unrecognized tags are kept verbatim rather than
	// guessed at.
	SchemeNoIntro Scheme = "no-intro"
	SchemeRedump  Scheme = "redump"
)

// ParseScheme validates s against the known schemes (case-insensitive); ""
// means SchemeNoTag, romctl's default.
func ParseScheme(s string) (Scheme, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "":
		return SchemeNoTag, nil
	case string(SchemeNoTag):
		return SchemeNoTag, nil
	case string(SchemeKeep):
		return SchemeKeep, nil
	case string(SchemeNoIntro), "nointro":
		return SchemeNoIntro, nil
	case string(SchemeRedump):
		return SchemeRedump, nil
	default:
		return "", fmt.Errorf("unknown naming scheme %q (want notag, keep, no-intro, or redump)", s)
	}
}

// ApplyScheme renames name (no extension) according to scheme. SchemeNoTag
// defers to CleanName unchanged; SchemeKeep returns name as-is; SchemeNoIntro
// and SchemeRedump reorder recognized Region/Disc N/Rev N tags into that
// scheme's own order and append anything else found, verbatim, at the end.
func ApplyScheme(name string, scheme Scheme) string {
	switch scheme {
	case SchemeKeep:
		return name
	case SchemeNoIntro, SchemeRedump:
		title, t := extractSchemeTags(name)
		title = bracketRE.ReplaceAllString(title, "")
		title = moveLeadingArticle(title)
		title = strings.ReplaceAll(title, " - the ", " - The ")
		title = spacesRE.ReplaceAllString(title, " ")
		title = strings.TrimSpace(title)
		return strings.TrimSpace(t.appendTo(scheme, title))
	default:
		return CleanName(name)
	}
}

var (
	discInnerRE     = regexp.MustCompile(`(?i)^Disc\s+\d+$`)
	revisionInnerRE = regexp.MustCompile(`(?i)^(rev\.?\s*\d+(\.\d+)?|v\d+(\.\d+)*)$`)
)

// regionNames are the region tags both No-Intro and Redump use (full
// country/aggregate names - no GoodTools-style single-letter codes, which
// neither convention uses).
var regionNames = []string{
	"USA", "Japan", "Europe", "World", "Asia", "Australia", "Brazil",
	"Canada", "China", "Denmark", "Finland", "France", "Germany", "Greece",
	"Hong Kong", "Ireland", "Italy", "Korea", "Netherlands", "Norway",
	"Poland", "Portugal", "Russia", "Spain", "Sweden", "Switzerland",
	"Taiwan", "UK", "Unknown",
}

var regionCanon = func() map[string]string {
	m := make(map[string]string, len(regionNames))
	for _, n := range regionNames {
		m[strings.ToLower(n)] = n
	}
	return m
}()

// canonicalRegions returns inner's comma-separated tokens in their
// canonical casing, or nil if any token isn't a recognized region name.
func canonicalRegions(inner string) []string {
	parts := strings.Split(inner, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		canon, ok := regionCanon[strings.ToLower(strings.TrimSpace(p))]
		if !ok {
			return nil
		}
		out = append(out, canon)
	}
	return out
}

// schemeTags holds the tags extractSchemeTags recognized out of a name,
// plus anything else it found but didn't - kept verbatim, original
// parenthetical and all, rather than guessed at.
type schemeTags struct {
	regions  []string
	disc     string // e.g. "Disc 1", without parens
	revision string // e.g. "Rev 1" or "v1.1", without parens
	other    []string
}

// extractSchemeTags removes every "(...)" group from name, classifying
// each as Region/Disc N/Rev N or, failing that, "other" (kept verbatim for
// appendTo to re-append later), and returns the bare title plus the result.
func extractSchemeTags(name string) (string, schemeTags) {
	matches := parenRE.FindAllStringIndex(name, -1)
	if matches == nil {
		return name, schemeTags{}
	}
	var t schemeTags
	var b strings.Builder
	last := 0
	discFound, revFound, regionFound := false, false, false
	for _, m := range matches {
		start, end := m[0], m[1]
		group := strings.TrimSpace(name[start:end])
		inner := strings.TrimSuffix(strings.TrimPrefix(group, "("), ")")
		switch {
		case !discFound && discInnerRE.MatchString(inner):
			t.disc = inner
			discFound = true
		case !revFound && revisionInnerRE.MatchString(inner):
			t.revision = inner
			revFound = true
		case !regionFound && canonicalRegions(inner) != nil:
			t.regions = canonicalRegions(inner)
			regionFound = true
		default:
			t.other = append(t.other, group)
		}
		b.WriteString(name[last:start])
		last = end
	}
	b.WriteString(name[last:])
	return b.String(), t
}

// appendTo rebuilds title with t's tags in scheme's own order: Region, then
// (for SchemeRedump) Disc N, then Rev N, then everything else verbatim.
// SchemeNoIntro has no Disc N slot, so a Disc N tag found on a scheme-
// no-intro name is treated as "else" rather than dropped.
func (t schemeTags) appendTo(scheme Scheme, title string) string {
	out := title
	if len(t.regions) > 0 {
		out += " (" + strings.Join(t.regions, ", ") + ")"
	}
	other := t.other
	if scheme == SchemeRedump && t.disc != "" {
		out += " (" + t.disc + ")"
	} else if t.disc != "" {
		other = append([]string{"(" + t.disc + ")"}, other...)
	}
	if t.revision != "" {
		out += " (" + t.revision + ")"
	}
	for _, o := range other {
		out += " " + o
	}
	return out
}
