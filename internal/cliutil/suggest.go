package cliutil

// Suggest returns the candidate closest to input by edit distance, for a
// "did you mean X?" hint on a typo'd command or system name. Returns ""
// when nothing is close enough to be worth suggesting, so a genuinely
// unrelated name doesn't get a misleading guess.
func Suggest(input string, candidates []string) string {
	best := ""
	bestDist := -1
	for _, c := range candidates {
		d := levenshtein(input, c)
		// Half the length of the shorter string (min 1, so two single-char
		// names still get a chance): a suggestion close enough to type by
		// accident, not just vaguely similar.
		limit := len(input)
		if len(c) < limit {
			limit = len(c)
		}
		limit = (limit + 1) / 2
		if limit < 1 {
			limit = 1
		}
		if d > limit {
			continue
		}
		if bestDist == -1 || d < bestDist {
			best, bestDist = c, d
		}
	}
	return best
}

// levenshtein returns the edit distance between a and b.
func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			del := prev[j] + 1
			ins := cur[j-1] + 1
			sub := prev[j-1] + cost
			min := del
			if ins < min {
				min = ins
			}
			if sub < min {
				min = sub
			}
			cur[j] = min
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}
