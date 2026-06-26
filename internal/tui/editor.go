package tui

import (
	"os"
	"os/exec"

	"pkm/internal/vault"

	tea "github.com/charmbracelet/bubbletea"
)

type editorFinishedMsg struct {
	note *vault.Note
	err  error
}

// openInEditor suspends the TUI, opens path in $EDITOR, and returns a Cmd
// that reloads the note after the editor exits.
func openInEditor(v *vault.Vault, n *vault.Note) tea.Cmd {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	c := exec.Command(editor, n.Path)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		if err != nil {
			return editorFinishedMsg{err: err}
		}
		reloaded, loadErr := v.Load(n.Path)
		return editorFinishedMsg{note: reloaded, err: loadErr}
	})
}
