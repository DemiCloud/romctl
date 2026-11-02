package actions

import (
	"regexp"
	"strings"
)

var (
	parenRE   = regexp.MustCompile(`\s*\([^)]*\)`)
	discRE    = regexp.MustCompile(`^\(Disc \d+\)$`)
	bracketRE = regexp.MustCompile(`\s*\[[^\]]*\]`)
	spacesRE  = regexp.MustCompile(`\s+`)
	articles  = []string{"The ", "An ", "A "}
)

// CleanName strips region/dump tags and moves a leading article to the end
// of the title, mirroring the Bash tool's clean_name (a Perl one-liner);
// Go's RE2 engine has no lookahead, so the article-move and "keep (Disc N)"
// steps are done with plain string logic instead of a single regex.
func CleanName(name string) string {
	name = stripParensExceptDisc(name)
	name = bracketRE.ReplaceAllString(name, "")
	name = moveLeadingArticle(name)
	name = strings.ReplaceAll(name, " - the ", " - The ")
	name = spacesRE.ReplaceAllString(name, " ")
	return strings.TrimSpace(name)
}

// stripParensExceptDisc removes every "(...)" group except one whose
// content is exactly "Disc N".
func stripParensExceptDisc(name string) string {
	matches := parenRE.FindAllStringIndex(name, -1)
	if matches == nil {
		return name
	}
	var b strings.Builder
	last := 0
	for _, m := range matches {
		start, end := m[0], m[1]
		group := strings.TrimSpace(name[start:end])
		if discRE.MatchString(group) {
			continue
		}
		b.WriteString(name[last:start])
		last = end
	}
	b.WriteString(name[last:])
	return b.String()
}

// moveLeadingArticle rewrites "The Foo - Bar" to "Foo, The - Bar" (and
// likewise for "A"/"An"), except when what follows the article is itself
// "- " (e.g. "The - Foo" is left alone).
func moveLeadingArticle(name string) string {
	for _, article := range articles {
		rest, ok := strings.CutPrefix(name, article)
		if !ok || strings.HasPrefix(rest, "- ") {
			continue
		}
		word := strings.TrimSpace(article)
		if idx := strings.Index(rest, " - "); idx >= 0 {
			return rest[:idx] + ", " + word + rest[idx:]
		}
		return rest + ", " + word
	}
	return name
}
