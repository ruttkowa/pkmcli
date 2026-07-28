package index

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"

	"pkm/internal/vault"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS notes (
	id      TEXT PRIMARY KEY,
	title   TEXT NOT NULL,
	state   TEXT NOT NULL,
	project TEXT,
	path    TEXT NOT NULL,
	created TEXT,
	updated TEXT
);

CREATE VIRTUAL TABLE IF NOT EXISTS notes_fts USING fts5(
	id UNINDEXED,
	title,
	body,
	tokenize='unicode61'
);

CREATE TABLE IF NOT EXISTS tags (
	note_id TEXT NOT NULL,
	tag     TEXT NOT NULL,
	PRIMARY KEY (note_id, tag)
);

CREATE TABLE IF NOT EXISTS links (
	src  TEXT NOT NULL,
	dest TEXT NOT NULL,
	PRIMARY KEY (src, dest)
);
`

type Index struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite index at pkmDir/.pkm/index.db.
func Open(pkmDir string) (*Index, error) {
	path := filepath.Join(pkmDir, "index.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open index: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}
	return &Index{db: db}, nil
}

func (idx *Index) Close() error {
	return idx.db.Close()
}

// Upsert inserts or replaces a note's metadata in the index.
func (idx *Index) Upsert(n *vault.Note) error {
	tx, err := idx.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := upsertNote(tx, n); err != nil {
		return err
	}
	return tx.Commit()
}

func upsertNote(tx *sql.Tx, n *vault.Note) error {
	_, err := tx.Exec(`
		INSERT OR REPLACE INTO notes (id, title, state, project, path, created, updated)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		n.ID, n.Title, string(n.State), n.Project, n.Path,
		n.Created.Format("2006-01-02T15:04:05"),
		n.Updated.Format("2006-01-02T15:04:05"),
	)
	if err != nil {
		return err
	}

	if _, err = tx.Exec(`DELETE FROM tags WHERE note_id = ?`, n.ID); err != nil {
		return err
	}
	for _, tag := range n.Tags {
		if _, err = tx.Exec(`INSERT OR IGNORE INTO tags (note_id, tag) VALUES (?, ?)`, n.ID, tag); err != nil {
			return err
		}
	}

	if _, err = tx.Exec(`DELETE FROM links WHERE src = ?`, n.ID); err != nil {
		return err
	}
	for _, dest := range extractLinks(n.Body) {
		if _, err = tx.Exec(`INSERT OR IGNORE INTO links (src, dest) VALUES (?, ?)`, n.ID, dest); err != nil {
			return err
		}
	}

	// FTS: delete existing entry by rowid, then reinsert
	tx.Exec(`DELETE FROM notes_fts WHERE rowid IN (SELECT rowid FROM notes_fts WHERE id = ?)`, n.ID)
	_, err = tx.Exec(`INSERT INTO notes_fts (id, title, body) VALUES (?, ?, ?)`, n.ID, n.Title, n.Body)
	if err != nil {
		return err
	}

	return nil
}

// Delete removes a note from the index by ID.
func (idx *Index) Delete(id string) error {
	tx, err := idx.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, q := range []string{
		`DELETE FROM notes WHERE id = ?`,
		`DELETE FROM tags WHERE note_id = ?`,
		`DELETE FROM links WHERE src = ?`,
		`DELETE FROM notes_fts WHERE rowid IN (SELECT rowid FROM notes_fts WHERE id = ?)`,
	} {
		if _, err := tx.Exec(q, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Search returns note IDs matching a full-text query.
func (idx *Index) Search(query string) ([]string, error) {
	rows, err := idx.db.Query(
		`SELECT id FROM notes_fts WHERE notes_fts MATCH ? ORDER BY rank`, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// CountByState returns the number of notes per state.
func (idx *Index) CountByState() (map[vault.NoteState]int, error) {
	rows, err := idx.db.Query(`SELECT state, COUNT(*) FROM notes GROUP BY state`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := make(map[vault.NoteState]int)
	for rows.Next() {
		var state string
		var count int
		if err := rows.Scan(&state, &count); err != nil {
			return nil, err
		}
		counts[vault.NoteState(state)] = count
	}
	return counts, rows.Err()
}

// SearchByTag returns note IDs that have the given tag.
func (idx *Index) SearchByTag(tag string) ([]string, error) {
	rows, err := idx.db.Query(`SELECT note_id FROM tags WHERE tag = ?`, strings.ToLower(tag))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// Backlinks returns IDs of notes that link to destTitle.
func (idx *Index) Backlinks(destTitle string) ([]string, error) {
	rows, err := idx.db.Query(`SELECT src FROM links WHERE dest = ?`, destTitle)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// RebuildAll drops and rebuilds the full index from the given notes.
func (idx *Index) RebuildAll(notes []*vault.Note) error {
	tx, err := idx.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, table := range []string{"notes", "notes_fts", "tags", "links"} {
		if _, err := tx.Exec(`DELETE FROM ` + table); err != nil {
			return err
		}
	}
	for _, n := range notes {
		if err := upsertNote(tx, n); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// extractLinks parses [[Title]] and [[Title|Alias]] wikilinks from body.
func extractLinks(body string) []string {
	var links []string
	for {
		start := strings.Index(body, "[[")
		if start == -1 {
			break
		}
		end := strings.Index(body[start:], "]]")
		if end == -1 {
			break
		}
		inner := body[start+2 : start+end]
		if pipe := strings.Index(inner, "|"); pipe != -1 {
			inner = inner[:pipe]
		}
		links = append(links, strings.TrimSpace(inner))
		body = body[start+end+2:]
	}
	return links
}
