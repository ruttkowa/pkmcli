package tui

import (
	"path/filepath"
	"strings"

	"pkm/internal/index"
	"pkm/internal/vault"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fsnotify/fsnotify"
)

type vaultChangedMsg struct {
	note *vault.Note
}

// StartWatcher starts a background fsnotify watcher on vault/notes/ and sends
// vaultChangedMsg to the Bubble Tea program whenever a .md file changes.
func StartWatcher(v *vault.Vault, idx *index.Index, p *tea.Program) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return
	}
	watcher.Add(v.NotesDir())

	go func() {
		defer watcher.Close()
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if !strings.HasSuffix(event.Name, ".md") {
					continue
				}
				switch {
				case event.Has(fsnotify.Write) || event.Has(fsnotify.Create):
					n, err := v.Load(filepath.Clean(event.Name))
					if err == nil {
						idx.Upsert(n)
						p.Send(vaultChangedMsg{note: n})
					}
				case event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename):
					// extract ID from filename "<ID> <Title>.md"
					base := filepath.Base(event.Name)
					if parts := strings.SplitN(base, " ", 2); len(parts) > 0 {
						idx.Delete(parts[0])
					}
					p.Send(vaultChangedMsg{})
				}
			case _, ok := <-watcher.Errors:
				if !ok {
					return
				}
			}
		}
	}()
}
