package tui

import (
	"sort"
	"strings"

	"pkm/internal/vault"
)

// searchHit is one fuzzy-search result, ranked by score (higher first).
type searchHit struct {
	note       *vault.Note
	score      int
	viaContent bool // matched the body, not the title
}

// titleMatchBonus keeps every title match ahead of every content-only match,
// regardless of fuzzy score.
const titleMatchBonus = 1000

// searchHits ranks notes against query: a fuzzy subsequence match on the
// title, or (failing that) a plain substring match on the body. limit <= 0
// means unbounded.
func searchHits(query string, notes []*vault.Note, limit int) []searchHit {
	ql := strings.ToLower(strings.TrimSpace(query))
	var hits []searchHit
	for _, n := range notes {
		if score, ok := fuzzyScore(ql, n.Title); ok {
			hits = append(hits, searchHit{note: n, score: score + titleMatchBonus})
			continue
		}
		if ql != "" && strings.Contains(strings.ToLower(n.Body), ql) {
			hits = append(hits, searchHit{note: n, viaContent: true})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
	if limit > 0 && len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}

// fuzzySearchNotes returns every matching note, ranked, for the :search
// command's full results view.
func fuzzySearchNotes(query string, notes []*vault.Note) []*vault.Note {
	hits := searchHits(query, notes, -1)
	out := make([]*vault.Note, len(hits))
	for i, h := range hits {
		out[i] = h.note
	}
	return out
}

// fuzzyScore reports whether every rune of query appears in target in order
// (case-insensitive), and a score rewarding early, contiguous matches — the
// classic fuzzy-finder subsequence heuristic. An empty query matches
// everything with score 0.
func fuzzyScore(query, target string) (int, bool) {
	q := []rune(strings.ToLower(query))
	if len(q) == 0 {
		return 0, true
	}
	t := []rune(strings.ToLower(target))

	qi := 0
	score := 0
	lastMatch := -2
	for ti := 0; ti < len(t) && qi < len(q); ti++ {
		if t[ti] != q[qi] {
			continue
		}
		if lastMatch == ti-1 {
			score += 3 // contiguous run
		} else {
			score += 1
		}
		if ti == 0 {
			score += 2 // prefix bonus
		}
		lastMatch = ti
		qi++
	}
	if qi < len(q) {
		return 0, false
	}
	return score, true
}
