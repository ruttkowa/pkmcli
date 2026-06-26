package vault

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const notesDir = "notes"

// Open returns a Vault rooted at path. Creates .pkm/ and notes/ if missing.
func Open(path string) (*Vault, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	for _, dir := range []string{
		filepath.Join(abs, notesDir),
		filepath.Join(abs, ".pkm"),
		filepath.Join(abs, "templates"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	return &Vault{Root: abs}, nil
}

// NotesDir returns the absolute path to vault/notes/.
func (v *Vault) NotesDir() string {
	return filepath.Join(v.Root, notesDir)
}

// GenerateID returns a timestamp-based note ID (YYYYMMDDHHmm).
func GenerateID() string {
	return time.Now().Format("200601021504")
}

// Filename returns the canonical filename for a note: "<ID> <Title>.md"
func Filename(id, title string) string {
	return fmt.Sprintf("%s %s.md", id, title)
}

// Create writes a new note to disk and returns it.
func (v *Vault) Create(title string) (*Note, error) {
	id := GenerateID()
	now := time.Now().Truncate(time.Second)
	n := &Note{
		ID:      id,
		Title:   title,
		Created: now,
		Updated: now,
		State:   StateInbox,
		Tags:    []string{},
	}
	n.Path = filepath.Join(v.NotesDir(), Filename(id, title))
	if body := v.ApplyTemplate(id, title, now); body != "" {
		n.Body = body
	}
	return n, v.Save(n)
}

// Save writes a note back to disk, updating the Updated timestamp.
func (v *Vault) Save(n *Note) error {
	n.Updated = time.Now().Truncate(time.Second)
	data, err := marshalNote(n)
	if err != nil {
		return err
	}
	return os.WriteFile(n.Path, data, 0o644)
}

// Load reads a single .md file and returns a Note.
func (v *Vault) Load(path string) (*Note, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	n := &Note{Path: path}
	if err := parseNote(data, n); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return n, nil
}

// ListAll returns all notes in vault/notes/.
func (v *Vault) ListAll() ([]*Note, error) {
	entries, err := os.ReadDir(v.NotesDir())
	if err != nil {
		return nil, err
	}
	var notes []*Note
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		n, err := v.Load(filepath.Join(v.NotesDir(), e.Name()))
		if err != nil {
			continue // skip malformed files
		}
		notes = append(notes, n)
	}
	return notes, nil
}

// ListByState returns notes matching the given state.
func (v *Vault) ListByState(state NoteState) ([]*Note, error) {
	all, err := v.ListAll()
	if err != nil {
		return nil, err
	}
	var out []*Note
	for _, n := range all {
		if n.State == state {
			out = append(out, n)
		}
	}
	return out, nil
}

// SetState updates a note's state and saves it.
func (v *Vault) SetState(n *Note, state NoteState) error {
	n.State = state
	return v.Save(n)
}

// FindByTitle returns the first note whose title contains the query (case-insensitive).
func (v *Vault) FindByTitle(query string) (*Note, error) {
	all, err := v.ListAll()
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(query)
	for _, n := range all {
		if strings.Contains(strings.ToLower(n.Title), q) {
			return n, nil
		}
	}
	return nil, fmt.Errorf("note not found: %q", query)
}
