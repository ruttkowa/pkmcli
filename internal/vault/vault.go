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
	ps, err := openProjectStore(abs)
	if err != nil {
		return nil, fmt.Errorf("project store: %w", err)
	}
	return &Vault{Root: abs, Projects: ps}, nil
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

// Import reads an external markdown file into the vault as a new note,
// preserving any existing frontmatter tags but always assigning a fresh
// ID/Created/Updated/State, since the source is unlikely to already match
// this vault's schema. If move is true, the source file is removed after a
// successful write; otherwise it's left in place (copy).
func (v *Vault) Import(srcPath string, state NoteState, move bool) (*Note, error) {
	raw, err := os.ReadFile(srcPath)
	if err != nil {
		return nil, err
	}
	n := &Note{}
	if err := parseNote(raw, n); err != nil {
		return nil, err
	}
	if n.Tags == nil {
		n.Tags = []string{}
	}

	title := strings.TrimSuffix(filepath.Base(srcPath), filepath.Ext(srcPath))
	now := time.Now().Truncate(time.Second)
	n.ID = GenerateID()
	n.Title = title
	n.Created = now
	n.Updated = now
	n.State = state
	n.Path = filepath.Join(v.NotesDir(), Filename(n.ID, title))

	if err := v.Save(n); err != nil {
		return nil, err
	}
	if move {
		if err := os.Remove(srcPath); err != nil {
			return nil, err
		}
	}
	return n, nil
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

// NormalizeNotes scans the flat notes directory and brings manually added
// Markdown files into the vault format. Valid notes are left byte-for-byte
// untouched; incomplete files receive fresh metadata, default to Inbox, and
// are renamed to the canonical "<ID> <Title>.md" form.
func (v *Vault) NormalizeNotes() ([]*Note, error) {
	entries, err := os.ReadDir(v.NotesDir())
	if err != nil {
		return nil, err
	}
	used := make(map[string]bool)
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".md") {
			if id := filenameID(e.Name()); id != "" {
				used[id] = true
			}
		}
	}

	var notes []*Note
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".md") {
			continue
		}
		path := filepath.Join(v.NotesDir(), e.Name())
		n, err := v.Load(path)
		if err != nil {
			return nil, err
		}
		incomplete := n.ID == "" || n.Title == "" || n.Created.IsZero() ||
			n.Updated.IsZero() || !validState(n.State)
		if n.Title == "" {
			n.Title = titleFromFilename(e.Name())
		}
		if n.ID == "" {
			n.ID = availableNoteID(used)
			used[n.ID] = true
		}
		if n.Created.IsZero() {
			n.Created = time.Now().Truncate(time.Second)
		}
		if n.Updated.IsZero() {
			n.Updated = n.Created
		}
		if !validState(n.State) {
			n.State = StateInbox
			n.Project = ""
		}
		if n.Tags == nil {
			n.Tags = []string{}
		}

		canonical := filepath.Join(v.NotesDir(), Filename(n.ID, n.Title))
		needsWrite := e.Name() != filepath.Base(canonical) || incomplete
		// Load again isn't necessary to detect missing metadata: a canonical
		// marshalled note always starts with frontmatter, while a manually
		// dropped plain file does not.
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil, readErr
		}
		needsWrite = needsWrite || !strings.HasPrefix(strings.TrimPrefix(string(raw), "\xef\xbb\xbf"), fmDelimiter)
		if needsWrite {
			oldPath := path
			n.Path = canonical
			if err := v.Save(n); err != nil {
				return nil, err
			}
			if oldPath != canonical {
				if err := os.Remove(oldPath); err != nil {
					return nil, err
				}
			}
		} else {
			n.Path = path
		}
		notes = append(notes, n)
	}
	return notes, nil
}

func filenameID(name string) string {
	first, _, ok := strings.Cut(strings.TrimSuffix(name, filepath.Ext(name)), " ")
	if !ok || len(first) != 12 {
		return ""
	}
	if _, err := time.Parse("200601021504", first); err != nil {
		return ""
	}
	return first
}

func titleFromFilename(name string) string {
	title := strings.TrimSuffix(name, filepath.Ext(name))
	if id := filenameID(name); id != "" {
		title = strings.TrimSpace(strings.TrimPrefix(title, id))
	}
	if title == "" {
		return "Untitled"
	}
	return title
}

func availableNoteID(used map[string]bool) string {
	now := time.Now()
	for offset := 0; ; offset++ {
		id := now.Add(time.Duration(offset) * time.Minute).Format("200601021504")
		if !used[id] {
			return id
		}
	}
}

func validState(state NoteState) bool {
	for _, candidate := range AllStates {
		if state == candidate {
			return true
		}
	}
	return false
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

// ListByTag returns all notes that carry the given tag (case-insensitive, no # prefix).
func (v *Vault) ListByTag(tag string) ([]*Note, error) {
	all, err := v.ListAll()
	if err != nil {
		return nil, err
	}
	tagLower := strings.ToLower(tag)
	var out []*Note
	for _, n := range all {
		for _, t := range n.Tags {
			if strings.ToLower(t) == tagLower {
				out = append(out, n)
				break
			}
		}
	}
	return out, nil
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
