package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type suggestion struct {
	cmd  string
	args string
	desc string
}

var allCommands = []suggestion{
	{"new", `"Title"`, "Create note in Inbox"},
	{"open", "<note>", "Open note by title"},
	{"search", "<query>  or  #tag", "Full-text / tag search"},
	{"move", `<note> → <state>`, "Promote or demote a note"},
	{"archive", "<note>", "Archive a note"},
	{"split", "[note]", "Open a new split pane"},
	{"close", "", "Close the focused pane"},
	{"theme", "dark|light", "Switch color theme"},
}

const maxSuggestions = 7

type paletteModel struct {
	input     string
	selected  int
	submitted bool
	cancelled bool
}

func newPalette() paletteModel                    { return paletteModel{} }
func newPaletteWithInput(input string) paletteModel { return paletteModel{input: input} }

func (p paletteModel) value() string { return strings.TrimSpace(p.input) }

// filteredSuggestions returns commands matching the current input.
func (p paletteModel) filteredSuggestions() []suggestion {
	trimmed := strings.TrimLeft(p.input, " ")
	if trimmed == "" {
		return allCommands
	}
	// Once the command word has been typed (contains a space), lock to that command.
	if idx := strings.Index(trimmed, " "); idx != -1 {
		first := trimmed[:idx]
		for _, c := range allCommands {
			if c.cmd == first {
				return []suggestion{c}
			}
		}
		return nil
	}
	var out []suggestion
	for _, c := range allCommands {
		if strings.HasPrefix(c.cmd, trimmed) {
			out = append(out, c)
		}
	}
	return out
}

// dropdownHeight returns the number of rows the suggestion dropdown will occupy.
func (p paletteModel) dropdownHeight() int {
	n := len(p.filteredSuggestions())
	if n > maxSuggestions {
		n = maxSuggestions
	}
	return n
}

func (p paletteModel) update(msg tea.KeyMsg) (paletteModel, tea.Cmd) {
	suggestions := p.filteredSuggestions()
	n := len(suggestions)
	if n > maxSuggestions {
		n = maxSuggestions
	}

	switch msg.String() {
	case "enter":
		p.submitted = true
	case "esc":
		p.cancelled = true
	case "tab", "right":
		// Complete the highlighted suggestion, add a trailing space.
		if len(suggestions) > 0 {
			sel := p.selected
			if sel >= len(suggestions) {
				sel = 0
			}
			p.input = suggestions[sel].cmd + " "
			p.selected = 0
		}
	case "up", "ctrl+p":
		if n > 0 {
			p.selected = (p.selected - 1 + n) % n
		}
	case "down", "ctrl+n":
		if n > 0 {
			p.selected = (p.selected + 1) % n
		}
	case "backspace", "ctrl+h":
		if len(p.input) > 0 {
			runes := []rune(p.input)
			p.input = string(runes[:len(runes)-1])
			p.selected = 0
		}
	case "ctrl+u":
		p.input = ""
		p.selected = 0
	default:
		if len(msg.Runes) == 1 {
			p.input += string(msg.Runes)
			p.selected = 0
		}
	}
	return p, nil
}

// renderDropdown renders suggestion rows to be placed just above the input line.
func (p paletteModel) renderDropdown(width int) string {
	suggestions := p.filteredSuggestions()
	if len(suggestions) == 0 {
		return ""
	}

	t := activeTheme
	selRow := lipgloss.NewStyle().
		Background(t.Accent).
		Foreground(t.AccentFg).
		Width(width)

	normRow := lipgloss.NewStyle().
		Background(t.DropdownBg).
		Foreground(t.TextPrimary).
		Width(width)

	boldCmd := lipgloss.NewStyle().Bold(true)
	mutedArgs := lipgloss.NewStyle().Foreground(t.TextSecond)
	mutedDesc := lipgloss.NewStyle().Foreground(t.TextDim)

	limit := len(suggestions)
	if limit > maxSuggestions {
		limit = maxSuggestions
	}

	rows := make([]string, limit)
	for i, s := range suggestions[:limit] {
		indicator := "  "
		if i == p.selected {
			indicator = "▶ "
		}
		line := indicator + boldCmd.Render(s.cmd)
		if s.args != "" {
			line += " " + mutedArgs.Render(s.args)
		}
		line += "   " + mutedDesc.Render(s.desc)

		if i == p.selected {
			rows[i] = selRow.Render(line)
		} else {
			rows[i] = normRow.Render(line)
		}
	}
	return strings.Join(rows, "\n")
}

// renderInputLine renders the ": input█" row shown at the bottom when command mode is active.
func (p paletteModel) renderInputLine(width int) string {
	t := activeTheme
	bg := t.DropdownBg
	prefix := lipgloss.NewStyle().
		Background(bg).
		Foreground(t.Cursor).
		Bold(true).
		Render(":")
	text := lipgloss.NewStyle().
		Background(bg).
		Foreground(t.AccentFg).
		Render(" " + p.input + "█")
	return lipgloss.NewStyle().
		Background(bg).
		Width(width).
		Render(prefix + text)
}
