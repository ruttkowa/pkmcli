package tui

import (
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
	tea "github.com/charmbracelet/bubbletea"
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
	{label: "Theme", options: []string{"Nord", "Solarized Dark", "Dracula", "Gruvbox", "Tokyo Night"}},
	{label: "Sidebar width", options: []string{"20%", "25%", "33%"}},
	{label: "Restore session", options: []string{"on", "off"}},
	{label: "Line numbers", options: []string{"on", "off"}},
}

// cfgLabelWidth is wide enough to cover the longest label ("Restore session" = 15) plus
// the "▶ " cursor prefix (2) and two spaces of padding.
const cfgLabelWidth = 20

// configSection is one tab of the config overlay.
type configSection int

const (
	secGeneral configSection = iota
	secKeybindings
	secVariables
)

var sectionNames = []string{"General", "Keybindings", "Variables"}

// varEditMode tracks whether the Variables section is idle or capturing text
// for a new/edited entry.
type varEditMode int

const (
	varModeNone varEditMode = iota
	varModeName
	varModeValue
)

type variableEntry struct {
	Name  string
	Value string
}

type configPane struct {
	section configSection

	// General section
	cursor int
	values []int // parallel to configItems; each is an index into item.options

	// Keybindings section
	kbCursor    int
	kbKeys      []string // parallel to keymapLabels
	kbCapturing bool

	// Variables section
	varCursor     int
	variables     []variableEntry // working copy, kept sorted by name
	varMode       varEditMode
	varTargetName string // name the pending edit will be saved under
	varNameInput  textinput.Model
	varValueInput textinput.Model
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

	var vars []variableEntry
	for name, value := range cfg.Variables {
		vars = append(vars, variableEntry{Name: name, Value: value})
	}
	sort.Slice(vars, func(i, j int) bool { return vars[i].Name < vars[j].Name })

	nameInput := textinput.New()
	nameInput.Prompt = ""
	nameInput.Placeholder = "name"
	applyTextInputTheme(&nameInput)

	valueInput := textinput.New()
	valueInput.Prompt = ""
	valueInput.Placeholder = "value"
	applyTextInputTheme(&valueInput)

	return configPane{
		values:        v,
		kbKeys:        keymapToSlice(cfg.Keymap),
		variables:     vars,
		varNameInput:  nameInput,
		varValueInput: valueInput,
	}
}

func (c configPane) nextSection() configPane {
	c.section = (c.section + 1) % configSection(len(sectionNames))
	return c
}

func (c configPane) prevSection() configPane {
	c.section = (c.section - 1 + configSection(len(sectionNames))) % configSection(len(sectionNames))
	return c
}

// busy reports whether the pane is mid-capture or mid-edit, so callers know
// not to switch sections or treat Esc as "close the overlay".
func (c configPane) busy() bool {
	return c.kbCapturing || c.varMode != varModeNone
}

// cancelInPlace undoes an in-progress capture/edit. Returns ok=true if it
// consumed the cancellation; ok=false means there was nothing to cancel and
// the caller (Esc handling) should close the overlay instead.
func (c configPane) cancelInPlace() (configPane, bool) {
	if c.kbCapturing {
		c.kbCapturing = false
		return c, true
	}
	if c.varMode != varModeNone {
		c.varMode = varModeNone
		c.varTargetName = ""
		c.varNameInput.SetValue("")
		c.varValueInput.SetValue("")
		return c, true
	}
	return c, false
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

// updateKeybindings handles input while the Keybindings section is active.
// Rebind keys are restricted to ctrl/alt chords — the whole point of this
// feature is dodging multiplexer prefixes, so binding a bare letter (which
// would silently shadow navigation elsewhere) is rejected.
func (c configPane) updateKeybindings(msg tea.KeyMsg) configPane {
	if c.kbCapturing {
		key := msg.String()
		if strings.HasPrefix(key, "ctrl+") || strings.HasPrefix(key, "alt+") {
			c.kbKeys[c.kbCursor] = key
			c.kbCapturing = false
		}
		return c
	}
	switch msg.String() {
	case "j", "down":
		c.kbCursor = (c.kbCursor + 1) % len(keymapLabels)
	case "k", "up":
		c.kbCursor = (c.kbCursor - 1 + len(keymapLabels)) % len(keymapLabels)
	case "enter":
		c.kbCapturing = true
	case "d":
		c.kbKeys[c.kbCursor] = keymapToSlice(defaultKeymap())[c.kbCursor]
	}
	return c
}

// updateVariables handles input while the Variables section is active.
// Renaming isn't a distinct flow — delete and re-add covers it with far less
// state to track.
func (c configPane) updateVariables(msg tea.KeyMsg) configPane {
	if c.varMode == varModeName {
		switch msg.String() {
		case "enter":
			name := strings.TrimSpace(c.varNameInput.Value())
			if name == "" {
				return c
			}
			c.varTargetName = name
			c.varMode = varModeValue
			c.varValueInput.SetValue("")
			c.varValueInput.Focus()
		default:
			c.varNameInput, _ = c.varNameInput.Update(msg)
		}
		return c
	}

	if c.varMode == varModeValue {
		switch msg.String() {
		case "enter":
			c = c.setVariable(c.varTargetName, c.varValueInput.Value())
			c.varMode = varModeNone
			c.varTargetName = ""
		default:
			c.varValueInput, _ = c.varValueInput.Update(msg)
		}
		return c
	}

	rows := len(c.variables) + 1 // +1 for the "Add variable" row
	switch msg.String() {
	case "j", "down":
		c.varCursor = (c.varCursor + 1) % rows
	case "k", "up":
		c.varCursor = (c.varCursor - 1 + rows) % rows
	case "enter":
		if c.varCursor == len(c.variables) {
			c.varNameInput.SetValue("")
			c.varMode = varModeName
			c.varNameInput.Focus()
		} else {
			entry := c.variables[c.varCursor]
			c.varTargetName = entry.Name
			c.varValueInput.SetValue(entry.Value)
			c.varMode = varModeValue
			c.varValueInput.Focus()
		}
	case "d", "x":
		if c.varCursor < len(c.variables) {
			c = c.removeVariable(c.variables[c.varCursor].Name)
			if c.varCursor >= len(c.variables) && c.varCursor > 0 {
				c.varCursor--
			}
		}
	}
	return c
}

func (c configPane) setVariable(name, value string) configPane {
	for i, e := range c.variables {
		if e.Name == name {
			c.variables[i].Value = value
			return c
		}
	}
	c.variables = append(c.variables, variableEntry{Name: name, Value: value})
	sort.Slice(c.variables, func(i, j int) bool { return c.variables[i].Name < c.variables[j].Name })
	return c
}

func (c configPane) removeVariable(name string) configPane {
	for i, e := range c.variables {
		if e.Name == name {
			c.variables = append(c.variables[:i], c.variables[i+1:]...)
			break
		}
	}
	return c
}

// variablesMap converts the working list back into the map form AppConfig stores.
func (c configPane) variablesMap() map[string]string {
	out := make(map[string]string, len(c.variables))
	for _, e := range c.variables {
		out[e.Name] = e.Value
	}
	return out
}

func (c configPane) render(width, height int) string {
	t := activeTheme

	heading := lipgloss.NewStyle().Bold(true).Foreground(t.Accent).Render("Configuration")

	tabs := make([]string, len(sectionNames))
	for i, name := range sectionNames {
		if configSection(i) == c.section {
			tabs[i] = lipgloss.NewStyle().Bold(true).Foreground(t.AccentFg).Background(t.Accent).Padding(0, 1).Render(name)
		} else {
			tabs[i] = lipgloss.NewStyle().Foreground(t.TextMuted).Padding(0, 1).Render(name)
		}
	}
	tabRow := strings.Join(tabs, " ")

	sep := lipgloss.NewStyle().Foreground(t.TextDim).Render(strings.Repeat("─", min(width-4, 36)))

	var rows []string
	var hint string
	switch c.section {
	case secKeybindings:
		rows, hint = c.renderKeybindings(t)
	case secVariables:
		rows, hint = c.renderVariables(t)
	default:
		rows, hint = c.renderGeneral(t)
	}

	lines := []string{heading, tabRow, sep, ""}
	lines = append(lines, rows...)
	lines = append(lines, "", hint)

	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		Padding(1, 2).
		Render(strings.Join(lines, "\n"))
}

func (c configPane) renderGeneral(t Theme) ([]string, string) {
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
	return rows, "[↑↓] select   [←→] change   [Tab] switch section   [Esc] save & close"
}

func (c configPane) renderKeybindings(t Theme) ([]string, string) {
	labelStyle := lipgloss.NewStyle().Width(cfgLabelWidth)

	var rows []string
	for i, label := range keymapLabels {
		key := c.kbKeys[i]
		if i == c.kbCursor {
			l := labelStyle.Bold(true).Foreground(t.TextPrimary).Render("▶ " + label)
			var v string
			if c.kbCapturing {
				v = lipgloss.NewStyle().Bold(true).Foreground(t.Cursor).Render("press ctrl/alt + a key…")
			} else {
				v = lipgloss.NewStyle().Bold(true).Foreground(t.Accent).Render(key)
			}
			rows = append(rows, l+v)
		} else {
			l := labelStyle.Foreground(t.TextMuted).Render("  " + label)
			v := lipgloss.NewStyle().Foreground(t.TextSecond).Render(key)
			rows = append(rows, l+v)
		}
	}
	return rows, "[↑↓] select   [Enter] rebind   [d] reset to default   [Tab] switch section   [Esc] save & close"
}

func (c configPane) renderVariables(t Theme) ([]string, string) {
	labelStyle := lipgloss.NewStyle().Width(cfgLabelWidth)

	var rows []string
	for i, e := range c.variables {
		selected := i == c.varCursor
		switch {
		case selected && c.varMode == varModeValue:
			l := labelStyle.Bold(true).Foreground(t.TextPrimary).Render("▶ " + e.Name)
			rows = append(rows, l+c.varValueInput.View())
		case selected:
			l := labelStyle.Bold(true).Foreground(t.TextPrimary).Render("▶ " + e.Name)
			v := lipgloss.NewStyle().Bold(true).Foreground(t.Accent).Render(e.Value)
			rows = append(rows, l+v)
		default:
			l := labelStyle.Foreground(t.TextMuted).Render("  " + e.Name)
			v := lipgloss.NewStyle().Foreground(t.TextSecond).Render(e.Value)
			rows = append(rows, l+v)
		}
	}

	onAddRow := c.varCursor == len(c.variables)
	switch {
	case onAddRow && c.varMode == varModeName:
		rows = append(rows, lipgloss.NewStyle().Bold(true).Foreground(t.TextPrimary).Render("▶ ")+c.varNameInput.View())
	case onAddRow && c.varMode == varModeValue:
		l := labelStyle.Bold(true).Foreground(t.TextPrimary).Render("▶ " + c.varTargetName)
		rows = append(rows, l+c.varValueInput.View())
	case onAddRow:
		rows = append(rows, lipgloss.NewStyle().Bold(true).Foreground(t.Accent).Render("▶ + Add variable"))
	default:
		rows = append(rows, lipgloss.NewStyle().Foreground(t.TextMuted).Render("  + Add variable"))
	}

	hint := "[↑↓] select   [Enter] add / edit value   [d] delete   [Tab] switch section   [Esc] save & close"
	return rows, hint
}
