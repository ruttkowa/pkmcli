package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	cfgItemTheme = iota
	cfgItemSidebarWidth
	cfgItemRestoreSession
	cfgItemLineNumbers
)

type configItem struct {
	label   string
	options []string
}

var configItems = []configItem{
	{label: "Theme",           options: []string{"Nord", "Solarized Dark", "Dracula", "Gruvbox", "Tokyo Night"}},
	{label: "Sidebar width",   options: []string{"20%", "25%", "33%"}},
	{label: "Restore session", options: []string{"on", "off"}},
	{label: "Line numbers",    options: []string{"on", "off"}},
}

// cfgLabelWidth is wide enough to cover the longest label ("Restore session" = 15) plus
// the "▶ " cursor prefix (2) and two spaces of padding.
const cfgLabelWidth = 20

type configPane struct {
	cursor int
	values []int // parallel to configItems; each is an index into item.options
}

func newConfigPane(cfg AppConfig) configPane {
	v := make([]int, len(configItems))

	for i, t := range ThemeChoices {
		if t.Name == cfg.Theme {
			v[cfgItemTheme] = i
			break
		}
	}

	switch cfg.SidebarWidth {
	case 20:
		v[cfgItemSidebarWidth] = 0
	case 33:
		v[cfgItemSidebarWidth] = 2
	default:
		v[cfgItemSidebarWidth] = 1 // 25%
	}

	if !cfg.RestoreSession {
		v[cfgItemRestoreSession] = 1
	}

	if !cfg.LineNumbers {
		v[cfgItemLineNumbers] = 1
	}

	return configPane{values: v}
}

func (c configPane) moveCursor(dir int) configPane {
	n := len(configItems)
	c.cursor = (c.cursor + dir + n) % n
	return c
}

func (c configPane) changeValue(dir int) configPane {
	n := len(configItems[c.cursor].options)
	c.values[c.cursor] = (c.values[c.cursor] + dir + n) % n
	return c
}

func (c configPane) render(width, height int) string {
	t := activeTheme

	heading := lipgloss.NewStyle().Bold(true).Foreground(t.Accent).Render("Configuration")
	sep := lipgloss.NewStyle().Foreground(t.TextDim).Render(strings.Repeat("─", min(width-4, 36)))

	labelStyle := lipgloss.NewStyle().Width(cfgLabelWidth)

	var rows []string
	for i, item := range configItems {
		val := item.options[c.values[i]]
		if i == c.cursor {
			label := labelStyle.Bold(true).Foreground(t.TextPrimary).Render("▶ " + item.label)
			valS := lipgloss.NewStyle().Bold(true).Foreground(t.Accent).Render("◀ " + val + " ▶")
			rows = append(rows, label+valS)
		} else {
			label := labelStyle.Foreground(t.TextMuted).Render("  " + item.label)
			valS := lipgloss.NewStyle().Foreground(t.TextSecond).Render(val)
			rows = append(rows, label+valS)
		}
	}

	hint := lipgloss.NewStyle().Foreground(t.TextDim).Render("[↑↓] select   [←→] change   [Esc] save & close")

	lines := []string{heading, sep, ""}
	lines = append(lines, rows...)
	lines = append(lines, "", hint)

	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		Padding(1, 2).
		Render(strings.Join(lines, "\n"))
}

