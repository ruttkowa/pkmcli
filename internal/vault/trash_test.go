package vault

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTrashMovesFileAndRecordsSidecarEntry(t *testing.T) {
	v := setupVault(t)
	n, err := v.Create("Doomed")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	n.State = StateProjects
	n.Project = "Homelab"
	if err := v.Save(n); err != nil {
		t.Fatalf("Save: %v", err)
	}
	origPath := n.Path

	if err := v.Trash(n); err != nil {
		t.Fatalf("Trash: %v", err)
	}

	if _, err := os.Stat(origPath); !os.IsNotExist(err) {
		t.Errorf("expected original file gone, stat err = %v", err)
	}
	if _, err := os.Stat(n.Path); err != nil {
		t.Errorf("expected file present at new trash path %q: %v", n.Path, err)
	}
	if filepath.Dir(n.Path) != trashDir(v.Root) {
		t.Errorf("trash path %q not under trash dir %q", n.Path, trashDir(v.Root))
	}

	entries, err := v.ListTrash()
	if err != nil {
		t.Fatalf("ListTrash: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 trash entry, got %d", len(entries))
	}
	e := entries[0]
	if e.ID != n.ID || e.Title != "Doomed" || e.State != string(StateProjects) || e.Project != "Homelab" {
		t.Errorf("entry = %+v, unexpected fields", e)
	}
	if e.OrigPath != origPath {
		t.Errorf("entry.OrigPath = %q, want %q", e.OrigPath, origPath)
	}
	if e.TrashPath != n.Path {
		t.Errorf("entry.TrashPath = %q, want %q", e.TrashPath, n.Path)
	}
	if e.DeletedAt.IsZero() {
		t.Error("entry.DeletedAt is zero")
	}
}

func TestTrashCollisionAppendsSuffix(t *testing.T) {
	v := setupVault(t)
	n, err := v.Create("Collides")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	base := filepath.Base(n.Path)

	// Something already occupies the exact trash path this note would land
	// at (e.g. a previous note with the same basename, already trashed).
	if err := os.MkdirAll(trashDir(v.Root), 0o755); err != nil {
		t.Fatalf("mkdir trash dir: %v", err)
	}
	preoccupied := filepath.Join(trashDir(v.Root), base)
	if err := os.WriteFile(preoccupied, []byte("dummy"), 0o644); err != nil {
		t.Fatalf("write collision file: %v", err)
	}

	if err := v.Trash(n); err != nil {
		t.Fatalf("Trash: %v", err)
	}
	if n.Path == preoccupied {
		t.Fatalf("expected a suffixed path distinct from the pre-occupied one, got the same: %q", n.Path)
	}
	if _, err := os.Stat(preoccupied); err != nil {
		t.Errorf("expected the pre-occupying file to survive untouched: %v", err)
	}
	if _, err := os.Stat(n.Path); err != nil {
		t.Errorf("expected the note's file at its suffixed trash path: %v", err)
	}
}

func TestRestoreReturnsNoteToOrigPathWithOldState(t *testing.T) {
	v := setupVault(t)
	if _, err := v.Projects.Create("Homelab"); err != nil {
		t.Fatalf("Projects.Create: %v", err)
	}
	n, err := v.Create("Restorable")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	n.State = StateProjects
	n.Project = "Homelab"
	n.Body = "some content"
	if err := v.Save(n); err != nil {
		t.Fatalf("Save: %v", err)
	}
	origPath := n.Path

	if err := v.Trash(n); err != nil {
		t.Fatalf("Trash: %v", err)
	}
	entries, _ := v.ListTrash()
	if len(entries) != 1 {
		t.Fatalf("expected 1 trash entry, got %d", len(entries))
	}

	restored, err := v.Restore(entries[0])
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if restored.Path != origPath {
		t.Errorf("restored.Path = %q, want %q", restored.Path, origPath)
	}
	if restored.State != StateProjects || restored.Project != "Homelab" {
		t.Errorf("restored State/Project = %q/%q, want projects/Homelab", restored.State, restored.Project)
	}
	if restored.Body != "some content\n" && restored.Body != "some content" {
		t.Errorf("restored.Body = %q, want the original content preserved", restored.Body)
	}
	if _, err := os.Stat(origPath); err != nil {
		t.Errorf("expected file back at orig path: %v", err)
	}

	remaining, _ := v.ListTrash()
	if len(remaining) != 0 {
		t.Errorf("expected trash empty after restore, got %d entries", len(remaining))
	}
}

func TestRestoreCollisionAtOrigPathUsesSuffixedName(t *testing.T) {
	v := setupVault(t)
	n, err := v.Create("Collider")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	origPath := n.Path
	if err := v.Trash(n); err != nil {
		t.Fatalf("Trash: %v", err)
	}
	entries, _ := v.ListTrash()

	// Something now occupies the original path (e.g. the user created a new
	// note with the same filename after deleting the old one).
	if err := os.WriteFile(origPath, []byte("---\nid: \"new\"\ntitle: New\n---\n"), 0o644); err != nil {
		t.Fatalf("write collider: %v", err)
	}

	restored, err := v.Restore(entries[0])
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if restored.Path == origPath {
		t.Errorf("expected a suffixed path distinct from the occupied orig path, got %q", restored.Path)
	}
	if filepath.Dir(restored.Path) != v.NotesDir() {
		t.Errorf("restored.Path %q not under notes dir", restored.Path)
	}
	if _, err := os.Stat(restored.Path); err != nil {
		t.Errorf("expected restored file to exist: %v", err)
	}
}

func TestRestoreFallsBackToInboxWhenProjectGone(t *testing.T) {
	v := setupVault(t)
	if _, err := v.Projects.Create("Homelab"); err != nil {
		t.Fatalf("Projects.Create: %v", err)
	}
	n, err := v.Create("Orphaned")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	n.State = StateProjects
	n.Project = "Homelab"
	if err := v.Save(n); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := v.Trash(n); err != nil {
		t.Fatalf("Trash: %v", err)
	}
	entries, _ := v.ListTrash()

	// The project was removed entirely while the note sat in trash — Get
	// must fail for a name no project store entry has, so simulate that by
	// restoring against a differently-named (nonexistent) project instead.
	entry := entries[0]
	entry.Project = "Nonexistent Project"

	restored, err := v.Restore(entry)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if restored.State != StateInbox || restored.Project != "" {
		t.Errorf("restored State/Project = %q/%q, want inbox/\"\" (project gone)", restored.State, restored.Project)
	}
}

func TestPurgeExpiredOnlyRemovesPastRetention(t *testing.T) {
	v := setupVault(t)
	nOld, err := v.Create("Old")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	nRecent, err := v.Create("Recent")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := v.Trash(nOld); err != nil {
		t.Fatalf("Trash old: %v", err)
	}
	if err := v.Trash(nRecent); err != nil {
		t.Fatalf("Trash recent: %v", err)
	}

	// Backdate the "Old" entry's DeletedAt past the retention window. Match
	// by title, not ID: GenerateID has minute granularity, so two notes
	// created back-to-back in the same test can share an ID.
	sc := readTrashSidecar(v.Root)
	for i := range sc.Entries {
		if sc.Entries[i].Title == "Old" {
			sc.Entries[i].DeletedAt = time.Now().AddDate(0, 0, -31)
		}
	}
	if err := writeTrashSidecar(v.Root, sc); err != nil {
		t.Fatalf("writeTrashSidecar: %v", err)
	}

	purged, err := v.PurgeExpired(30)
	if err != nil {
		t.Fatalf("PurgeExpired: %v", err)
	}
	if purged != 1 {
		t.Fatalf("purged = %d, want 1", purged)
	}

	remaining, _ := v.ListTrash()
	if len(remaining) != 1 || remaining[0].Title != "Recent" {
		t.Fatalf("remaining entries = %+v, want only Recent", remaining)
	}
	if _, err := os.Stat(nOld.Path); !os.IsNotExist(err) {
		t.Errorf("expected old trashed file removed, stat err = %v", err)
	}
	if _, err := os.Stat(nRecent.Path); err != nil {
		t.Errorf("expected recent trashed file to survive: %v", err)
	}
}

func TestPurgeExpiredNonPositiveRetentionFallsBackToDefault(t *testing.T) {
	v := setupVault(t)
	n, err := v.Create("Just Trashed")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := v.Trash(n); err != nil {
		t.Fatalf("Trash: %v", err)
	}

	// retentionDays <= 0 must not mean "purge everything immediately".
	purged, err := v.PurgeExpired(0)
	if err != nil {
		t.Fatalf("PurgeExpired: %v", err)
	}
	if purged != 0 {
		t.Fatalf("purged = %d, want 0 (a freshly trashed note must survive a <=0 retention value)", purged)
	}
	remaining, _ := v.ListTrash()
	if len(remaining) != 1 {
		t.Fatalf("expected the entry to survive, got %d remaining", len(remaining))
	}
}

func TestRemoveTrashEntryDeletesFileAndEntry(t *testing.T) {
	v := setupVault(t)
	n, err := v.Create("Undo Target")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := v.Trash(n); err != nil {
		t.Fatalf("Trash: %v", err)
	}
	trashPath := n.Path

	if err := v.RemoveTrashEntry(n.ID); err != nil {
		t.Fatalf("RemoveTrashEntry: %v", err)
	}
	if _, err := os.Stat(trashPath); !os.IsNotExist(err) {
		t.Errorf("expected trash file removed, stat err = %v", err)
	}
	entries, _ := v.ListTrash()
	if len(entries) != 0 {
		t.Errorf("expected trash empty, got %d entries", len(entries))
	}
}

func TestRemoveTrashEntryUnknownIDIsNoop(t *testing.T) {
	v := setupVault(t)
	if err := v.RemoveTrashEntry("nonexistent-id"); err != nil {
		t.Errorf("expected no error for an unknown ID, got %v", err)
	}
}
