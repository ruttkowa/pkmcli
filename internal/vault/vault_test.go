package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func setupVault(t *testing.T) *Vault {
	t.Helper()
	v, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return v
}

// --- frontmatter ---

func TestFrontmatterRoundTrip(t *testing.T) {
	now := time.Date(2026, 6, 24, 15, 30, 0, 0, time.UTC)
	original := &Note{
		ID:      "202606241530",
		Title:   "Docker",
		Created: now,
		Updated: now,
		State:   StateResearch,
		Tags:    []string{"linux", "containers"},
		Body:    "Some body text.\n",
	}

	data, err := marshalNote(original)
	if err != nil {
		t.Fatalf("marshalNote: %v", err)
	}

	parsed := &Note{}
	if err := parseNote(data, parsed); err != nil {
		t.Fatalf("parseNote: %v", err)
	}

	if parsed.ID != original.ID {
		t.Errorf("ID: got %q want %q", parsed.ID, original.ID)
	}
	if parsed.Title != original.Title {
		t.Errorf("Title: got %q want %q", parsed.Title, original.Title)
	}
	if parsed.State != original.State {
		t.Errorf("State: got %q want %q", parsed.State, original.State)
	}
	if len(parsed.Tags) != 2 || parsed.Tags[0] != "linux" {
		t.Errorf("Tags: got %v", parsed.Tags)
	}
	if strings.TrimSpace(parsed.Body) != strings.TrimSpace(original.Body) {
		t.Errorf("Body: got %q want %q", parsed.Body, original.Body)
	}
}

func TestParseMissingFrontmatter(t *testing.T) {
	raw := []byte("Just a plain body.\n")
	n := &Note{}
	if err := parseNote(raw, n); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(n.Body, "Just a plain body") {
		t.Errorf("Body not set, got %q", n.Body)
	}
}

func TestParseUnclosedFrontmatter(t *testing.T) {
	raw := []byte("---\ntitle: Oops\n")
	n := &Note{}
	err := parseNote(raw, n)
	if err == nil {
		t.Error("expected error for unclosed frontmatter")
	}
}

// --- vault CRUD ---

func TestCreateNote(t *testing.T) {
	v := setupVault(t)
	n, err := v.Create("My Note")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if n.Title != "My Note" {
		t.Errorf("Title: %q", n.Title)
	}
	if n.State != StateInbox {
		t.Errorf("State: %q", n.State)
	}
	if n.ID == "" {
		t.Error("ID is empty")
	}
	if _, err := os.Stat(n.Path); err != nil {
		t.Errorf("file not created: %v", err)
	}
}

func TestImportMovesFileByDefault(t *testing.T) {
	v := setupVault(t)
	src := filepath.Join(t.TempDir(), "External Note.md")
	if err := os.WriteFile(src, []byte("some external content"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	n, err := v.Import(src, StateInbox, true)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if n.Title != "External Note" {
		t.Errorf("Title: got %q, want %q", n.Title, "External Note")
	}
	if n.State != StateInbox {
		t.Errorf("State: got %q, want %q", n.State, StateInbox)
	}
	if n.ID == "" {
		t.Error("ID is empty")
	}
	if !strings.Contains(n.Body, "some external content") {
		t.Errorf("Body missing source content: %q", n.Body)
	}
	if _, err := os.Stat(n.Path); err != nil {
		t.Errorf("imported file not created: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("expected source file removed after move, stat err = %v", err)
	}
}

func TestImportCopiesFileWhenMoveFalse(t *testing.T) {
	v := setupVault(t)
	src := filepath.Join(t.TempDir(), "Keep Me.md")
	if err := os.WriteFile(src, []byte("copy me"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	n, err := v.Import(src, StateInbox, false)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if _, err := os.Stat(n.Path); err != nil {
		t.Errorf("imported file not created: %v", err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Errorf("expected source file preserved after copy, stat err = %v", err)
	}
}

func TestImportPreservesExistingTagsButAssignsFreshMetadata(t *testing.T) {
	v := setupVault(t)
	src := filepath.Join(t.TempDir(), "Tagged.md")
	raw := "---\ntags:\n  - foo\n  - bar\nstate: archive\n---\nbody text\n"
	if err := os.WriteFile(src, []byte(raw), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	n, err := v.Import(src, StateProjects, true)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(n.Tags) != 2 || n.Tags[0] != "foo" || n.Tags[1] != "bar" {
		t.Errorf("Tags not preserved: %v", n.Tags)
	}
	// State/ID/Created/Updated must reflect the import call, not the
	// source file's own frontmatter (which claimed state: archive).
	if n.State != StateProjects {
		t.Errorf("State: got %q, want overridden to %q", n.State, StateProjects)
	}
}

func TestLoadNote(t *testing.T) {
	v := setupVault(t)
	created, _ := v.Create("Load Me")
	loaded, err := v.Load(created.Path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Title != "Load Me" {
		t.Errorf("Title: %q", loaded.Title)
	}
}

func TestSaveNote(t *testing.T) {
	v := setupVault(t)
	n, _ := v.Create("Save Me")
	n.Tags = []string{"go", "test"}
	if err := v.Save(n); err != nil {
		t.Fatalf("Save: %v", err)
	}
	reloaded, _ := v.Load(n.Path)
	if len(reloaded.Tags) != 2 || reloaded.Tags[0] != "go" {
		t.Errorf("Tags not persisted: %v", reloaded.Tags)
	}
}

func TestSaveLoadCycleDoesNotAccumulateLeadingBlankLines(t *testing.T) {
	v := setupVault(t)
	n, _ := v.Create("Repeated Toggle")
	n.Body = "- [ ] Task A"
	if err := v.Save(n); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Simulate repeated checkbox-toggle round trips: load, re-save unchanged, reload.
	for i := 0; i < 5; i++ {
		reloaded, err := v.Load(n.Path)
		if err != nil {
			t.Fatalf("Load (cycle %d): %v", i, err)
		}
		if strings.HasPrefix(reloaded.Body, "\n") {
			t.Fatalf("cycle %d: Body gained a leading blank line: %q", i, reloaded.Body)
		}
		if reloaded.Body != "- [ ] Task A" {
			t.Fatalf("cycle %d: Body corrupted: %q", i, reloaded.Body)
		}
		if err := v.Save(reloaded); err != nil {
			t.Fatalf("Save (cycle %d): %v", i, err)
		}
	}
}

func TestListAll(t *testing.T) {
	v := setupVault(t)
	v.Create("A")
	v.Create("B")
	v.Create("C")

	notes, err := v.ListAll()
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(notes) != 3 {
		t.Errorf("count: got %d want 3", len(notes))
	}
}

func TestListByState(t *testing.T) {
	v := setupVault(t)
	n1, _ := v.Create("Inbox One")
	n2, _ := v.Create("Inbox Two")
	n3, _ := v.Create("Research One")
	v.SetState(n3, StateResearch)
	_ = n1
	_ = n2

	inbox, _ := v.ListByState(StateInbox)
	if len(inbox) != 2 {
		t.Errorf("inbox count: got %d want 2", len(inbox))
	}
	research, _ := v.ListByState(StateResearch)
	if len(research) != 1 {
		t.Errorf("research count: got %d want 1", len(research))
	}
}

func TestSetState(t *testing.T) {
	v := setupVault(t)
	n, _ := v.Create("Move Me")
	if err := v.SetState(n, StateProjects); err != nil {
		t.Fatalf("SetState: %v", err)
	}
	reloaded, _ := v.Load(n.Path)
	if reloaded.State != StateProjects {
		t.Errorf("State: got %q want %q", reloaded.State, StateProjects)
	}
}

func TestFindByTitle(t *testing.T) {
	v := setupVault(t)
	v.Create("Docker Basics")
	v.Create("Kubernetes Setup")

	n, err := v.FindByTitle("docker")
	if err != nil {
		t.Fatalf("FindByTitle: %v", err)
	}
	if n.Title != "Docker Basics" {
		t.Errorf("Title: %q", n.Title)
	}

	_, err = v.FindByTitle("notexist")
	if err == nil {
		t.Error("expected error for missing note")
	}
}

// --- templates ---

func TestApplyTemplate(t *testing.T) {
	v := setupVault(t)
	tmplDir := filepath.Join(v.Root, "templates")
	os.MkdirAll(tmplDir, 0o755)
	os.WriteFile(filepath.Join(tmplDir, "default.md"), []byte(
		"# {{title}}\nID: {{id}}\nCreated: {{created}}\n",
	), 0o644)

	now := time.Now()
	body := v.ApplyTemplate("202606241530", "Docker", now)
	if !strings.Contains(body, "# Docker") {
		t.Errorf("title not substituted: %q", body)
	}
	if !strings.Contains(body, "202606241530") {
		t.Errorf("id not substituted: %q", body)
	}
}

func TestApplyTemplateNoTemplates(t *testing.T) {
	v := setupVault(t)
	body := v.ApplyTemplate("123", "X", time.Now())
	if body != "" {
		t.Errorf("expected empty string with no templates, got %q", body)
	}
}
