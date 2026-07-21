package tui

import (
	"os"
	"strings"

	"pkm/internal/vault"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// exportField is one focus stop in the :export popover.
type exportField int

const (
	expFldPath exportField = iota
	expFldConfirm
	expFldCount
)

// exportPane is the popover overlay for exporting the currently open note's
// raw markdown (frontmatter included, byte-identical to the vault copy) to a
// path outside the vault. Modeled on importPane, but smaller: export is
// always a copy (never touches the vault note), so there is no move/copy
// toggle and no destination-state field.
type exportPane struct {
	focused exportField

	pathInput   textinput.Model
	suggestions []string
	suggestSel  int

	// pendingOverwritePath is set after the user confirms once that an
	// existing target may be overwritten. It only "arms" a further Enter on
	// the CONFIRM button for that exact path — any edit to the path field
	// clears it, so a stale confirmation can't silently apply to a
	// different, unreviewed target.
	pendingOverwritePath string

	confirmed bool
	cancelled bool
	errMsg    string
}

func newExportPane(n *vault.Note) exportPane {
	pi := textinput.New()
	pi.Prompt = ""
	pi.Placeholder = "path/to/export.md"
	applyTextInputTheme(&pi)
	pi.Focus()

	if n != nil {
		name := vault.Filename(n.ID, n.Title)
		pi.SetValue(name)
		pi.CursorEnd()
	}

	p := exportPane{pathInput: pi}
	p.suggestions = pathSuggestions(p.pathInput.Value())
	return p
}

func (p exportPane) cycleField(dir int) exportPane {
	n := int(expFldCount)
	p.focused = exportField((int(p.focused) + dir + n) % n)
	return p
}

func (p exportPane) update(msg tea.KeyMsg) exportPane {
	switch msg.String() {
	case "esc":
		p.cancelled = true
		return p
	case "tab":
		return p.cycleField(1)
	case "shift+tab":
		return p.cycleField(-1)
	}

	switch p.focused {
	case expFldPath:
		switch msg.String() {
		case "up":
			if n := len(p.suggestions); n > 0 {
				p.suggestSel = (p.suggestSel - 1 + n) % n
			}
		case "down":
			if n := len(p.suggestions); n > 0 {
				p.suggestSel = (p.suggestSel + 1) % n
			}
		case "enter":
			if p.suggestSel < len(p.suggestions) {
				p.pathInput.SetValue(p.suggestions[p.suggestSel])
				p.pathInput.CursorEnd()
				p.suggestions = pathSuggestions(p.pathInput.Value())
				p.suggestSel = 0
				p.pendingOverwritePath = ""
				p.errMsg = ""
			}
		default:
			p.pathInput, _ = p.pathInput.Update(msg)
			p.suggestions = pathSuggestions(p.pathInput.Value())
			p.suggestSel = 0
			p.pendingOverwritePath = ""
			p.errMsg = ""
		}
	case expFldConfirm:
		if msg.String() == "enter" {
			p.confirmed = true
		}
	}
	return p
}

func (p exportPane) render(width, height int) string {
	t := activeTheme

	heading := lipgloss.NewStyle().Bold(true).Foreground(t.Accent).Render("Export note")
	sep := lipgloss.NewStyle().Foreground(t.TextDim).Render(strings.Repeat("─", min(width-4, 40)))

	labelStyle := lipgloss.NewStyle().Width(12)
	lbl := func(field exportField, label string) string {
		if p.focused == field {
			return labelStyle.Bold(true).Foreground(t.TextPrimary).Render("▶ " + label)
		}
		return labelStyle.Foreground(t.TextMuted).Render("  " + label)
	}

	pathRow := lbl(expFldPath, "Path:") + p.pathInput.View()

	confirmLabel := "[ Export ]"
	if p.focused == expFldConfirm {
		confirmLabel = lipgloss.NewStyle().Bold(true).Foreground(t.AccentFg).Background(t.Accent).Padding(0, 1).Render("Export")
	} else {
		confirmLabel = lipgloss.NewStyle().Foreground(t.TextMuted).Padding(0, 1).Render(confirmLabel)
	}

	lines := []string{heading, sep, "", pathRow}

	if p.focused == expFldPath && len(p.suggestions) > 0 {
		for i, s := range p.suggestions {
			sty := lipgloss.NewStyle().Width(width - 4).Background(t.DropdownBg).Foreground(t.TextPrimary)
			indicator := "    "
			if i == p.suggestSel {
				sty = sty.Background(t.Accent).Foreground(t.AccentFg)
				indicator = "  ▶ "
			}
			lines = append(lines, sty.Render(indicator+s))
		}
	}

	lines = append(lines, "", confirmLabel)
	if p.errMsg != "" {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(t.Cursor).Render(p.errMsg))
	}
	lines = append(lines, "", lipgloss.NewStyle().Foreground(t.TextDim).Render("[Tab] next field   [Shift+Tab] prev   [Enter] confirm/complete   [Esc] cancel"))

	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		Padding(1, 2).
		Render(strings.Join(lines, "\n"))
}

// expandExportPath resolves a leading "~" (or bare "~") to the user's home
// directory. Neither pathSuggestions nor vault.Import expand it today —
// this is scoped to export's own final path resolution, not a shared helper,
// so :import's existing (also tilde-less) behavior is left untouched.
func expandExportPath(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
		return path
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return home + path[1:]
		}
	}
	return path
}
