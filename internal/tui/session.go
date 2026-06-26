package tui

import (
	"os"
	"path/filepath"

	"pkm/internal/vault"

	"gopkg.in/yaml.v3"
)

type sessionData struct {
	LastNoteID  string `yaml:"last_note_id,omitempty"`
	ActiveState string `yaml:"active_state,omitempty"`
	Theme       string `yaml:"theme,omitempty"`
}

func sessionPath(v *vault.Vault) string {
	return filepath.Join(v.Root, ".pkm", "session.yaml")
}

func saveSession(v *vault.Vault, m *Model) {
	s := sessionData{
		ActiveState: string(m.sidebar.activeState),
		Theme:       activeTheme.Name,
	}
	sp := m.splits[m.activeSplit]
	if sp.viewer.note != nil {
		s.LastNoteID = sp.viewer.note.ID
	}
	data, err := yaml.Marshal(&s)
	if err != nil {
		return
	}
	os.WriteFile(sessionPath(v), data, 0o644)
}

func loadSession(v *vault.Vault) sessionData {
	data, err := os.ReadFile(sessionPath(v))
	if err != nil {
		return sessionData{}
	}
	var s sessionData
	yaml.Unmarshal(data, &s)
	return s
}
