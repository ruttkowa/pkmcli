package tui

import (
	"testing"

	"pkm/internal/vault"
)

func TestFuzzyScorePartialAndOrder(t *testing.T) {
	if _, ok := fuzzyScore("doc", "Docker Basics"); !ok {
		t.Error("expected partial prefix \"doc\" to match \"Docker Basics\"")
	}
	if _, ok := fuzzyScore("dkr", "Docker Basics"); !ok {
		t.Error("expected out-of-order-but-in-sequence \"dkr\" to match \"Docker Basics\"")
	}
	if _, ok := fuzzyScore("xyz", "Docker Basics"); ok {
		t.Error("expected \"xyz\" not to match \"Docker Basics\"")
	}
	if _, ok := fuzzyScore("", "anything"); !ok {
		t.Error("expected empty query to match")
	}
	// Punctuation and partial words must not error or behave oddly — this
	// is the FTS pitfall the in-memory matcher exists to avoid.
	if _, ok := fuzzyScore("c++", "c++ notes"); !ok {
		t.Error("expected punctuation in the query to match literally, not error")
	}
}

func TestFuzzyScoreRanksContiguousMatchesHigher(t *testing.T) {
	contiguous, _ := fuzzyScore("doc", "Docker")
	scattered, _ := fuzzyScore("dor", "Docker")
	if contiguous <= scattered {
		t.Errorf("expected contiguous match to score higher: contiguous=%d scattered=%d", contiguous, scattered)
	}
}

func TestSearchHitsTitleBeatsContentOnly(t *testing.T) {
	titleHit := note("1", "Docker Basics")
	titleHit.Body = "nothing relevant here"
	contentHit := note("2", "Unrelated")
	contentHit.Body = "this note mentions docker in passing"

	hits := searchHits("docker", []*vault.Note{titleHit, contentHit}, 0)
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits, got %d", len(hits))
	}
	if hits[0].note.Title != "Docker Basics" || hits[0].viaContent {
		t.Errorf("expected title match ranked first, got %+v", hits[0])
	}
	if !hits[1].viaContent {
		t.Errorf("expected second hit to be a content-only match, got %+v", hits[1])
	}
}
