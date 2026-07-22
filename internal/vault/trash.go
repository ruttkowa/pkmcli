package vault

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	trashDirName     = "trash"
	trashSidecarName = "trash.json"
	trashSidecarVer  = 1
	DefaultRetention = 30
)

// TrashEntry records one trashed note's durable metadata — everything
// Restore needs that the file's own frontmatter doesn't authoritatively
// carry once it's sitting outside the vault's normal note flow.
type TrashEntry struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	State     string    `json:"state"`
	Project   string    `json:"project"`
	OrigPath  string    `json:"orig_path"`
	TrashPath string    `json:"trash_path"`
	DeletedAt time.Time `json:"deleted_at"`
}

type trashSidecar struct {
	Version int          `json:"version"`
	Entries []TrashEntry `json:"entries"`
}

func trashDir(vaultRoot string) string {
	return filepath.Join(vaultRoot, ".pkm", trashDirName)
}

func trashSidecarPath(vaultRoot string) string {
	return filepath.Join(vaultRoot, ".pkm", trashSidecarName)
}

// readTrashSidecar loads the sidecar, tolerating a missing or corrupt file
// by returning an empty list rather than an error — losing the trash index
// must never block startup or normal vault use.
func readTrashSidecar(vaultRoot string) trashSidecar {
	data, err := os.ReadFile(trashSidecarPath(vaultRoot))
	if err != nil {
		return trashSidecar{Version: trashSidecarVer}
	}
	var sc trashSidecar
	if err := json.Unmarshal(data, &sc); err != nil {
		return trashSidecar{Version: trashSidecarVer}
	}
	return sc
}

// writeTrashSidecar writes atomically (temp file + rename) since the
// background file watcher runs concurrently with the TUI's own writes.
func writeTrashSidecar(vaultRoot string, sc trashSidecar) error {
	sc.Version = trashSidecarVer
	data, err := json.MarshalIndent(&sc, "", "  ")
	if err != nil {
		return err
	}
	path := trashSidecarPath(vaultRoot)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// uniquePath appends "-1", "-2", ... before the extension until it finds a
// path that doesn't exist yet, so a same-named file already at dir doesn't
// get silently overwritten by a move.
func uniquePath(dir, filename string) string {
	candidate := filepath.Join(dir, filename)
	if _, err := os.Stat(candidate); os.IsNotExist(err) {
		return candidate
	}
	ext := filepath.Ext(filename)
	base := strings.TrimSuffix(filename, ext)
	for i := 1; ; i++ {
		candidate = filepath.Join(dir, fmt.Sprintf("%s-%d%s", base, i, ext))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}

// Trash moves a note's file into <vault>/.pkm/trash/ and records a sidecar
// entry, instead of permanently removing it (supersedes the old
// os.Remove-based Delete). n.Path is left pointing at the file's new
// location on success.
func (v *Vault) Trash(n *Note) error {
	dir := trashDir(v.Root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	trashPath := uniquePath(dir, filepath.Base(n.Path))
	if err := os.Rename(n.Path, trashPath); err != nil {
		return err
	}
	origPath := n.Path
	n.Path = trashPath

	sc := readTrashSidecar(v.Root)
	sc.Entries = append(sc.Entries, TrashEntry{
		ID:        n.ID,
		Title:     n.Title,
		State:     string(n.State),
		Project:   n.Project,
		OrigPath:  origPath,
		TrashPath: trashPath,
		DeletedAt: time.Now().Truncate(time.Second),
	})
	return writeTrashSidecar(v.Root, sc)
}

// ListTrash returns every trashed note's sidecar entry.
func (v *Vault) ListTrash() ([]TrashEntry, error) {
	return readTrashSidecar(v.Root).Entries, nil
}

// RemoveTrashEntry deletes a trashed note's file and sidecar entry without
// restoring it — used both by :trash's permanent-delete action and by
// undoing a :delete (Ctrl+Z recreates the note at its original path via the
// normal undo-stack Save, so the leftover trash copy would otherwise become
// an orphan: the same note existing twice, with the trashed copy later
// surfacing in :trash as a ghost of a note that's already back).
func (v *Vault) RemoveTrashEntry(id string) error {
	sc := readTrashSidecar(v.Root)
	idx := -1
	for i, e := range sc.Entries {
		if e.ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		return nil // nothing to remove — undoing a delete that predates trash, or already cleaned up
	}
	entry := sc.Entries[idx]
	if err := removeUnderTrashDir(v.Root, entry.TrashPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	sc.Entries = append(sc.Entries[:idx], sc.Entries[idx+1:]...)
	return writeTrashSidecar(v.Root, sc)
}

// removeUnderTrashDir deletes path, refusing if it doesn't resolve to
// somewhere inside <vault>/.pkm/trash/ — a defensive check so a corrupt or
// tampered sidecar entry can never make PurgeExpired/RemoveTrashEntry
// os.Remove an arbitrary path outside the trash directory.
func removeUnderTrashDir(vaultRoot, path string) error {
	dir := trashDir(vaultRoot)
	rel, err := filepath.Rel(dir, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("refusing to remove path outside trash dir: %s", path)
	}
	return os.Remove(path)
}

// Restore moves a trashed note back into the vault, restoring the State and
// Project it had when deleted (from the sidecar entry, the durable record —
// the file's own frontmatter was never rewritten by Trash, but the entry is
// the authoritative source restore validates against). If OrigPath is
// already occupied, it lands in notes/ under a collision-suffixed name
// instead. If the project no longer exists, it falls back to Inbox with no
// project — restoring never creates a project (a still-existing project
// can't violate the max-4-active limit either, since re-attaching a note
// doesn't create a new one).
func (v *Vault) Restore(e TrashEntry) (*Note, error) {
	destPath := e.OrigPath
	if _, err := os.Stat(destPath); err == nil {
		destPath = uniquePath(v.NotesDir(), filepath.Base(e.OrigPath))
	}
	if err := os.Rename(e.TrashPath, destPath); err != nil {
		return nil, err
	}

	n, err := v.Load(destPath)
	if err != nil {
		return nil, err
	}

	state := NoteState(e.State)
	project := e.Project
	if project != "" {
		if _, ok := v.Projects.Get(project); !ok {
			state, project = StateInbox, ""
		}
	}
	if state == StateProjects && project == "" {
		state = StateInbox
	}
	n.State = state
	n.Project = project
	if err := v.Save(n); err != nil {
		return nil, err
	}

	sc := readTrashSidecar(v.Root)
	for i, entry := range sc.Entries {
		if entry.ID == e.ID && entry.TrashPath == e.TrashPath {
			sc.Entries = append(sc.Entries[:i], sc.Entries[i+1:]...)
			break
		}
	}
	if err := writeTrashSidecar(v.Root, sc); err != nil {
		return nil, err
	}
	return n, nil
}

// PurgeExpired permanently deletes trashed notes older than retentionDays,
// called once at startup (no timer/background polling). retentionDays <= 0
// falls back to DefaultRetention rather than purging immediately — an
// unset/misconfigured value must never mean "delete everything now".
func (v *Vault) PurgeExpired(retentionDays int) (int, error) {
	if retentionDays <= 0 {
		retentionDays = DefaultRetention
	}
	cutoff := time.Now().AddDate(0, 0, -retentionDays)

	sc := readTrashSidecar(v.Root)
	var kept []TrashEntry
	purged := 0
	for _, e := range sc.Entries {
		if e.DeletedAt.After(cutoff) {
			kept = append(kept, e)
			continue
		}
		if err := removeUnderTrashDir(v.Root, e.TrashPath); err != nil && !os.IsNotExist(err) {
			kept = append(kept, e) // couldn't remove the file — keep the entry rather than lose track of it
			continue
		}
		purged++
	}
	sc.Entries = kept
	if err := writeTrashSidecar(v.Root, sc); err != nil {
		return purged, err
	}
	return purged, nil
}
