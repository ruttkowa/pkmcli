package index

import (
	"path/filepath"
	"testing"
	"time"

	"pkm/internal/vault"
)

func setupIndex(t *testing.T) (*Index, func()) {
	t.Helper()
	dir := t.TempDir()
	idx, err := Open(dir)
	if err != nil {
		t.Fatalf("Open index: %v", err)
	}
	return idx, func() { idx.Close() }
}

func makeNote(id, title string, state vault.NoteState, tags []string, body string) *vault.Note {
	now := time.Date(2026, 6, 24, 15, 30, 0, 0, time.UTC)
	return &vault.Note{
		ID:      id,
		Title:   title,
		State:   state,
		Tags:    tags,
		Body:    body,
		Created: now,
		Updated: now,
		Path:    filepath.Join("/tmp", id+" "+title+".md"),
	}
}

func TestUpsertAndFTSSearch(t *testing.T) {
	idx, close := setupIndex(t)
	defer close()

	n := makeNote("001", "Docker Basics", vault.StateInbox, nil, "Install docker on linux.")
	if err := idx.Upsert(n); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	ids, err := idx.Search("docker")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(ids) != 1 || ids[0] != "001" {
		t.Errorf("Search result: %v", ids)
	}
}

func TestSearchNoResults(t *testing.T) {
	idx, close := setupIndex(t)
	defer close()

	ids, err := idx.Search("notexist")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("expected no results, got %v", ids)
	}
}

func TestSearchByTag(t *testing.T) {
	idx, close := setupIndex(t)
	defer close()

	n1 := makeNote("001", "Docker", vault.StateInbox, []string{"linux", "containers"}, "")
	n2 := makeNote("002", "Kubernetes", vault.StateInbox, []string{"linux", "k8s"}, "")
	idx.Upsert(n1)
	idx.Upsert(n2)

	ids, err := idx.SearchByTag("linux")
	if err != nil {
		t.Fatalf("SearchByTag: %v", err)
	}
	if len(ids) != 2 {
		t.Errorf("expected 2, got %d: %v", len(ids), ids)
	}

	ids, _ = idx.SearchByTag("k8s")
	if len(ids) != 1 || ids[0] != "002" {
		t.Errorf("k8s tag: %v", ids)
	}
}

func TestCountByState(t *testing.T) {
	idx, close := setupIndex(t)
	defer close()

	idx.Upsert(makeNote("001", "A", vault.StateInbox, nil, ""))
	idx.Upsert(makeNote("002", "B", vault.StateInbox, nil, ""))
	idx.Upsert(makeNote("003", "C", vault.StateResearch, nil, ""))

	counts, err := idx.CountByState()
	if err != nil {
		t.Fatalf("CountByState: %v", err)
	}
	if counts[vault.StateInbox] != 2 {
		t.Errorf("inbox: got %d want 2", counts[vault.StateInbox])
	}
	if counts[vault.StateResearch] != 1 {
		t.Errorf("research: got %d want 1", counts[vault.StateResearch])
	}
}

func TestDelete(t *testing.T) {
	idx, close := setupIndex(t)
	defer close()

	n := makeNote("001", "Delete Me", vault.StateInbox, []string{"tag"}, "body text")
	idx.Upsert(n)
	idx.Delete("001")

	ids, _ := idx.Search("body")
	if len(ids) != 0 {
		t.Errorf("expected deleted note to not appear in search, got %v", ids)
	}
	counts, _ := idx.CountByState()
	if counts[vault.StateInbox] != 0 {
		t.Errorf("count after delete: %d", counts[vault.StateInbox])
	}
}

func TestUpsertUpdatesExisting(t *testing.T) {
	idx, close := setupIndex(t)
	defer close()

	n := makeNote("001", "Original", vault.StateInbox, nil, "first body")
	idx.Upsert(n)

	n.Title = "Updated"
	n.State = vault.StateResearch
	n.Body = "second body"
	idx.Upsert(n)

	counts, _ := idx.CountByState()
	if counts[vault.StateInbox] != 0 {
		t.Errorf("inbox should be 0 after update, got %d", counts[vault.StateInbox])
	}
	if counts[vault.StateResearch] != 1 {
		t.Errorf("research should be 1 after update, got %d", counts[vault.StateResearch])
	}

	ids, _ := idx.Search("second")
	if len(ids) != 1 {
		t.Errorf("new body not indexed: %v", ids)
	}
}

func TestBacklinks(t *testing.T) {
	idx, close := setupIndex(t)
	defer close()

	docker := makeNote("001", "Docker", vault.StateInbox, nil, "")
	linux := makeNote("002", "Linux", vault.StateInbox, nil, "See [[Docker]] for containers.")
	idx.Upsert(docker)
	idx.Upsert(linux)

	ids, err := idx.Backlinks("Docker")
	if err != nil {
		t.Fatalf("Backlinks: %v", err)
	}
	if len(ids) != 1 || ids[0] != "002" {
		t.Errorf("Backlinks: %v", ids)
	}
}

// --- extractLinks (pure function) ---

func TestExtractLinks(t *testing.T) {
	cases := []struct {
		body  string
		links []string
	}{
		{"no links here", nil},
		{"[[Docker]]", []string{"Docker"}},
		{"[[Docker|Container Runtime]]", []string{"Docker"}},
		{"[[A]] and [[B|Alias]]", []string{"A", "B"}},
		{"[[unclosed", nil},
	}

	for _, tc := range cases {
		got := extractLinks(tc.body)
		if len(got) != len(tc.links) {
			t.Errorf("body=%q: got %v want %v", tc.body, got, tc.links)
			continue
		}
		for i, l := range tc.links {
			if got[i] != l {
				t.Errorf("body=%q link[%d]: got %q want %q", tc.body, i, got[i], l)
			}
		}
	}
}
