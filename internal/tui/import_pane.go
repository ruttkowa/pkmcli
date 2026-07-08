package tui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"pkm/internal/vault"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// importField is one focus stop in the :import popover.
type importField int

const (
	impFldPath importField = iota
	impFldMove
	impFldDest
	impFldConfirm
	impFldCount
)

const importSuggestMax = 6

// importPane is the popover overlay for importing an external markdown file
// into the vault (see todo.md item 1). It mirrors configPane's structure:
// a self-contained model with its own update/render, shown full-pane via
// Model.showImport the same way Model.showConfig works.
type importPane struct {
	focused importField

	pathInput   textinput.Model
	suggestions []string
	suggestSel  int

	move     bool // true = move (default), false = copy
	destIdx  int  // index into vault.AllStates
	confirmed bool
	cancelled bool
	errMsg    string
}

func newImportPane() importPane {
	pi := textinput.New()
	pi.Prompt = ""
	pi.Placeholder = "path/to/note.md"
	applyTextInputTheme(&pi)
	pi.Focus()

	destIdx := 0
	for i, s := range vault.AllStates {
		if s == vault.StateInbox {
			destIdx = i
			break
		}
	}

	p := importPane{
		pathInput: pi,
		move:      true,
		destIdx:   destIdx,
	}
	p.suggestions = pathSuggestions(p.pathInput.Value())
	return p
}

// pathSuggestions lists directory entries matching the fragment's basename
// prefix in its directory, directories and .md files only, sorted, mirroring
// shell tab-completion. Directories are suffixed with "/" so a further
// keystroke can descend into them.
func pathSuggestions(fragment string) []string {
	dir := "."
	base := fragment
	if fragment == "" {
		dir = "."
		base = ""
	} else if strings.HasSuffix(fragment, string(os.PathSeparator)) {
		dir = fragment
		base = ""
	} else {
		dir = filepath.Dir(fragment)
		base = filepath.Base(fragment)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	baseLower := strings.ToLower(base)
	var out []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") && !strings.HasPrefix(base, ".") {
			continue
		}
		if !e.IsDir() && !strings.HasSuffix(strings.ToLower(name), ".md") {
			continue
		}
		if base != "" && !strings.HasPrefix(strings.ToLower(name), baseLower) {
			continue
		}
		full := filepath.Join(dir, name)
		if e.IsDir() {
			full += string(os.PathSeparator)
		}
		out = append(out, full)
	}
	sort.Strings(out)
	if len(out) > importSuggestMax {
		out = out[:importSuggestMax]
	}
	return out
}

func (p importPane) cycleField(dir int) importPane {
	n := int(impFldCount)
	p.focused = importField((int(p.focused) + dir + n) % n)
	return p
}

func (p importPane) update(msg tea.KeyMsg) importPane {
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
	case impFldPath:
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
			}
		default:
			p.pathInput, _ = p.pathInput.Update(msg)
			p.suggestions = pathSuggestions(p.pathInput.Value())
			p.suggestSel = 0
		}
	case impFldMove:
		if msg.String() == " " {
			p.move = !p.move
		}
	case impFldDest:
		n := len(vault.AllStates)
		switch msg.String() {
		case "left", "h":
			p.destIdx = (p.destIdx + n - 1) % n
		case "right", "l", "enter":
			p.destIdx = (p.destIdx + 1) % n
		}
	case impFldConfirm:
		if msg.String() == "enter" {
			p.confirmed = true
		}
	}
	return p
}

func (p importPane) render(width, height int) string {
	t := activeTheme

	heading := lipgloss.NewStyle().Bold(true).Foreground(t.Accent).Render("Import markdown file")
	sep := lipgloss.NewStyle().Foreground(t.TextDim).Render(strings.Repeat("─", min(width-4, 40)))

	labelStyle := lipgloss.NewStyle().Width(12)
	lbl := func(field importField, label string) string {
		if p.focused == field {
			return labelStyle.Bold(true).Foreground(t.TextPrimary).Render("▶ " + label)
		}
		return labelStyle.Foreground(t.TextMuted).Render("  " + label)
	}

	pathRow := lbl(impFldPath, "Path:") + p.pathInput.View()

	moveLabel := "○ Copy   ● Move"
	if !p.move {
		moveLabel = "● Copy   ○ Move"
	}
	moveStyle := lipgloss.NewStyle().Foreground(t.TextSecond)
	if p.focused == impFldMove {
		moveStyle = lipgloss.NewStyle().Bold(true).Foreground(t.Accent)
	}
	moveRow := lbl(impFldMove, "Mode:") + moveStyle.Render(moveLabel) + lipgloss.NewStyle().Foreground(t.TextDim).Render("  (space to toggle)")

	destRow := lbl(impFldDest, "Destination:") + renderStateSelector(vault.AllStates[p.destIdx], p.focused == impFldDest)

	confirmLabel := "[ Import ]"
	if p.focused == impFldConfirm {
		confirmLabel = lipgloss.NewStyle().Bold(true).Foreground(t.AccentFg).Background(t.Accent).Padding(0, 1).Render("Import")
	} else {
		confirmLabel = lipgloss.NewStyle().Foreground(t.TextMuted).Padding(0, 1).Render(confirmLabel)
	}

	lines := []string{heading, sep, "", pathRow}

	if p.focused == impFldPath && len(p.suggestions) > 0 {
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

	lines = append(lines, "", moveRow, destRow, "", confirmLabel)
	if p.errMsg != "" {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(t.Cursor).Render(p.errMsg))
	}
	lines = append(lines, "", lipgloss.NewStyle().Foreground(t.TextDim).Render("[Tab] next field   [Shift+Tab] prev   [Space] toggle mode   [Enter] confirm/complete   [Esc] cancel"))

	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		Padding(1, 2).
		Render(strings.Join(lines, "\n"))
}
