package tui

import (
	"fmt"
	"strings"

	"pkm/internal/vault"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type noteListModel struct {
	v      *vault.Vault
	notes  []*vault.Note
	cursor int
	chosen *vault.Note
}

func newNoteList(v *vault.Vault) noteListModel {
	return noteListModel{v: v}
}

func (m noteListModel) withNotes(notes []*vault.Note) noteListModel {
	m.notes = notes
	m.cursor = 0
	m.chosen = nil
	return m
}

func (m noteListModel) update(msg tea.KeyMsg) (noteListModel, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if m.cursor < len(m.notes)-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "enter":
		if len(m.notes) > 0 {
			m.chosen = m.notes[m.cursor]
		}
	}
	return m, nil
}

func (m noteListModel) render(width, height int, focused bool) string {
	if len(m.notes) == 0 {
		return lipgloss.NewStyle().
			Width(width).
			Height(height).
			Background(activeTheme.Bg).
			Foreground(activeTheme.TextDim).
			Align(lipgloss.Center, lipgloss.Center).
			Render("No notes here.\n\n:new \"Title\" to create one.")
	}

	t := activeTheme
	// Focused pane: accent cursor. Unfocused: dimmed cursor so
	// the position is still visible but doesn't compete with the active pane.
	activeFocused := lipgloss.NewStyle().
		Background(t.Accent).
		Foreground(t.AccentFg).
		Bold(true).
		Width(width - 2).
		Padding(0, 1)

	activeBlurred := lipgloss.NewStyle().
		Background(t.BlurredBg).
		Foreground(t.TextPrimary).
		Width(width - 2).
		Padding(0, 1)

	normalStyle := lipgloss.NewStyle().
		Width(width - 2).
		Background(t.Bg).
		Foreground(t.TextPrimary).
		Padding(0, 1)

	var b strings.Builder
	for i, n := range m.notes {
		tags := ""
		if len(n.Tags) > 0 {
			tags = lipgloss.NewStyle().
				Foreground(t.TextMuted).
				Render("  #" + strings.Join(n.Tags, " #"))
		}
		label := fmt.Sprintf("%-30s", truncate(n.Title, 30))
		if i == m.cursor {
			if focused {
				b.WriteString(activeFocused.Render(label) + tags + "\n")
			} else {
				b.WriteString(activeBlurred.Render(label) + tags + "\n")
			}
		} else {
			b.WriteString(normalStyle.Render(label) + tags + "\n")
		}
	}

	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		Background(activeTheme.Bg).
		Padding(1, 1).
		Render(b.String())
}

func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-1]) + "…"
}
