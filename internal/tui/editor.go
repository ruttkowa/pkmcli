package tui

import (
	"strings"

	"pkm/internal/vault"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// editHeaderRows is the number of rows consumed by the header above the textarea.
const editHeaderRows = 3

type editPane struct {
	note      *vault.Note
	ta        textarea.Model
	saved     bool
	cancelled bool
}

// newEditPane creates an in-pane editor for note n, sized to fit width×height.
// Returns the pane and the focus command (starts cursor blinking).
func newEditPane(n *vault.Note, width, height int) (editPane, tea.Cmd) {
	ta := textarea.New()
	ta.Prompt = " "
	ta.ShowLineNumbers = false
	ta.CharLimit = 0

	contentH := height - editHeaderRows
	if contentH < 1 {
		contentH = 1
	}
	// SetWidth/SetHeight must be called after Prompt is set.
	ta.SetWidth(width)
	ta.SetHeight(contentH)

	applyEditTheme(&ta)

	// SetValue positions cursor at end; move to beginning (below frontmatter).
	ta.SetValue(n.Body)
	ta, _ = ta.Update(tea.KeyMsg{Type: tea.KeyCtrlHome})

	focusCmd := ta.Focus()
	return editPane{note: n, ta: ta}, focusCmd
}

// applyEditTheme styles the textarea to match the active app theme.
func applyEditTheme(ta *textarea.Model) {
	bg := activeTheme.Bg

	ta.FocusedStyle = textarea.Style{
		Base:             lipgloss.NewStyle().Background(bg),
		CursorLine:       lipgloss.NewStyle().Background(activeTheme.BlurredBg),
		CursorLineNumber: lipgloss.NewStyle().Foreground(activeTheme.TextDim).Background(bg),
		EndOfBuffer:      lipgloss.NewStyle().Foreground(activeTheme.TextDim).Background(bg),
		LineNumber:       lipgloss.NewStyle().Foreground(activeTheme.TextDim).Background(bg),
		Placeholder:      lipgloss.NewStyle().Foreground(activeTheme.TextMuted).Background(bg),
		Prompt:           lipgloss.NewStyle().Foreground(activeTheme.TextDim).Background(bg),
		Text:             lipgloss.NewStyle().Foreground(activeTheme.TextPrimary).Background(bg),
	}
	ta.BlurredStyle = textarea.Style{
		Base:             lipgloss.NewStyle().Background(bg),
		CursorLine:       lipgloss.NewStyle().Background(bg),
		CursorLineNumber: lipgloss.NewStyle().Foreground(activeTheme.TextDim).Background(bg),
		EndOfBuffer:      lipgloss.NewStyle().Foreground(activeTheme.TextDim).Background(bg),
		LineNumber:       lipgloss.NewStyle().Foreground(activeTheme.TextDim).Background(bg),
		Placeholder:      lipgloss.NewStyle().Foreground(activeTheme.TextMuted).Background(bg),
		Prompt:           lipgloss.NewStyle().Foreground(activeTheme.TextDim).Background(bg),
		Text:             lipgloss.NewStyle().Foreground(activeTheme.TextMuted).Background(bg),
	}
}

// update routes all messages to the textarea, intercepting Ctrl+S and Esc.
func (e editPane) update(msg tea.Msg) (editPane, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "ctrl+s":
			e.saved = true
			return e, nil
		case "esc":
			e.cancelled = true
			return e, nil
		}
	}
	var cmd tea.Cmd
	e.ta, cmd = e.ta.Update(msg)
	return e, cmd
}

// render produces the pane content: a read-only metadata header above the textarea.
func (e editPane) render(width, height int) string {
	n := e.note
	bg := activeTheme.Bg

	titleLine := lipgloss.NewStyle().
		Bold(true).
		Foreground(activeTheme.TextPrimary).
		Background(bg).
		Width(width).
		Render(" " + n.Title)

	meta := " " + string(n.State)
	if len(n.Tags) > 0 {
		meta += "  #" + strings.Join(n.Tags, " #")
	}
	metaLine := lipgloss.NewStyle().
		Foreground(activeTheme.TextMuted).
		Background(bg).
		Width(width).
		Render(meta)

	sep := lipgloss.NewStyle().
		Foreground(activeTheme.BorderNormal).
		Background(bg).
		Render(strings.Repeat("─", width))

	return lipgloss.JoinVertical(lipgloss.Left,
		titleLine,
		metaLine,
		sep,
		e.ta.View(),
	)
}
