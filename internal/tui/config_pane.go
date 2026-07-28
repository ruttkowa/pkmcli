package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
)

const (
	cfgItemTheme = iota
	cfgItemSidebarWidth
	cfgItemRestoreSession
	cfgItemLineNumbers
	cfgItemShowTasksNav
	cfgItemShowTemplatesNav
	cfgItemTrashRetention
)

type configItem struct {
	label   string
	options []string
}

var configItems = []configItem{
	{label: "Theme", options: []string{"Nord", "Solarized Dark", "Dracula", "Gruvbox", "Tokyo Night", "Solarized Light", "Catppuccin Mocha", "Everforest"}},
	{label: "Sidebar width", options: []string{"20%", "25%", "33%"}},
	{label: "Restore session", options: []string{"on", "off"}},
	{label: "Line numbers", options: []string{"on", "off"}},
	{label: "Show Tasks nav", options: []string{"on", "off"}},
	{label: "Show Templates nav", options: []string{"on", "off"}},
	{label: "Trash retention", options: []string{"7 days", "14 days", "30 days", "60 days", "90 days"}},
}

// trashRetentionDaysOptions is the numeric value (#1's TrashRetentionDays)
// backing each configItems[cfgItemTrashRetention].options entry, at the
// same index — this General-section row reuses the existing fixed-choice
// cycling UI (every other row already works this way) rather than
// introducing a free-numeric-input widget the codebase has no precedent
// for, while still storing/producing a plain integer day count.
var trashRetentionDaysOptions = []int{7, 14, 30, 60, 90}

// cfgLabelWidth is wide enough to cover the longest label ("Show Templates nav" = 19) plus
// the "▶ " cursor prefix (2) and two spaces of padding.
const cfgLabelWidth = 23

// configSection is one tab of the config overlay.
type configSection int

const (
	secGeneral configSection = iota
	secKeybindings
	secVariables
	secIssues
	secBackup
)

var sectionNames = []string{"General", "Keys", "Variables", "Issues", "Backup"}

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

type issueEditMode int

const (
	issueModeNone issueEditMode = iota
	issueModeURL
	issueModeProject
)

var backupModes = []string{"off", "remote", "path"}
var backupIntervals = []int{0, 15, 60, 240}
var backupTimeouts = []int{30, 60, 120}

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

	// Issues section
	issueCursor    int // URL=0, projects=1..n, add row=n+1
	issueMode      issueEditMode
	issueTarget    int
	gitlabURL      string
	gitlabProjects []string
	issueInput     textinput.Model

	// Backup section
	backupCursor      int
	backupMode        int
	backupDestination string
	backupInterval    int
	backupTimeout     int
	backupEditing     bool
	backupInput       textinput.Model
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

	if !cfg.ShowTasksNav {
		v[cfgItemShowTasksNav] = 1
	}

	if !cfg.ShowTemplatesNav {
		v[cfgItemShowTemplatesNav] = 1
	}

	v[cfgItemTrashRetention] = 2 // default: 30 days
	for i, d := range trashRetentionDaysOptions {
		if d == cfg.TrashRetentionDays {
			v[cfgItemTrashRetention] = i
			break
		}
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

	issueInput := textinput.New()
	issueInput.Prompt = ""
	issueInput.Placeholder = "group/repo"
	applyTextInputTheme(&issueInput)

	backupInput := textinput.New()
	backupInput.Prompt = ""
	backupInput.Placeholder = "git URL or backup directory"
	applyTextInputTheme(&backupInput)

	backupMode := indexOfString(backupModes, cfg.BackupMode)
	backupInterval := indexOfInt(backupIntervals, cfg.BackupInterval)
	backupTimeout := indexOfInt(backupTimeouts, cfg.BackupTimeout)

	return configPane{
		values:            v,
		kbKeys:            keymapToSlice(cfg.Keymap),
		variables:         vars,
		varNameInput:      nameInput,
		varValueInput:     valueInput,
		gitlabURL:         cfg.GitLabURL,
		gitlabProjects:    append([]string(nil), cfg.GitLabProjects...),
		issueInput:        issueInput,
		backupMode:        backupMode,
		backupDestination: cfg.BackupDestination,
		backupInterval:    backupInterval,
		backupTimeout:     backupTimeout,
		backupInput:       backupInput,
	}
}

func indexOfString(values []string, value string) int {
	for i, candidate := range values {
		if candidate == value {
			return i
		}
	}
	return 0
}

func indexOfInt(values []int, value int) int {
	for i, candidate := range values {
		if candidate == value {
			return i
		}
	}
	return 0
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
	return c.kbCapturing || c.varMode != varModeNone || c.issueMode != issueModeNone || c.backupEditing
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
	if c.issueMode != issueModeNone {
		c.issueMode = issueModeNone
		c.issueInput.SetValue("")
		return c, true
	}
	if c.backupEditing {
		c.backupEditing = false
		c.backupInput.SetValue("")
		return c, true
	}
	return c, false
}

func (c configPane) updateBackup(msg tea.KeyMsg) configPane {
	if c.backupEditing {
		if msg.String() == "enter" {
			c.backupDestination = strings.TrimSpace(c.backupInput.Value())
			c.backupEditing = false
			c.backupInput.SetValue("")
		} else {
			c.backupInput, _ = c.backupInput.Update(msg)
		}
		return c
	}
	switch msg.String() {
	case "j", "down":
		c.backupCursor = (c.backupCursor + 1) % 4
	case "k", "up":
		c.backupCursor = (c.backupCursor + 3) % 4
	case "left", "h":
		c = c.changeBackupValue(-1)
	case "right", "l":
		c = c.changeBackupValue(1)
	case "enter":
		if c.backupCursor == 1 {
			c.backupInput.SetValue(c.backupDestination)
			c.backupInput.Focus()
			c.backupEditing = true
		} else {
			c = c.changeBackupValue(1)
		}
	}
	return c
}

func (c configPane) changeBackupValue(direction int) configPane {
	switch c.backupCursor {
	case 0:
		c.backupMode = (c.backupMode + direction + len(backupModes)) % len(backupModes)
	case 2:
		c.backupInterval = (c.backupInterval + direction + len(backupIntervals)) % len(backupIntervals)
	case 3:
		c.backupTimeout = (c.backupTimeout + direction + len(backupTimeouts)) % len(backupTimeouts)
	}
	return c
}

func (c configPane) updateIssues(msg tea.KeyMsg) configPane {
	if c.issueMode != issueModeNone {
		if msg.String() == "enter" {
			value := strings.TrimSpace(c.issueInput.Value())
			if value != "" {
				if c.issueMode == issueModeURL {
					c.gitlabURL = strings.TrimRight(value, "/")
				} else if c.issueTarget >= 0 && c.issueTarget < len(c.gitlabProjects) {
					c.gitlabProjects[c.issueTarget] = value
				} else {
					c.gitlabProjects = append(c.gitlabProjects, value)
					c.issueCursor = len(c.gitlabProjects)
				}
			}
			c.issueMode = issueModeNone
			c.issueInput.SetValue("")
		} else {
			c.issueInput, _ = c.issueInput.Update(msg)
		}
		return c
	}
	rows := len(c.gitlabProjects) + 2
	switch msg.String() {
	case "j", "down":
		c.issueCursor = (c.issueCursor + 1) % rows
	case "k", "up":
		c.issueCursor = (c.issueCursor - 1 + rows) % rows
	case "a", "n":
		c.issueTarget = len(c.gitlabProjects)
		c.issueMode = issueModeProject
		c.issueInput.SetValue("")
		c.issueInput.Focus()
	case "enter":
		switch {
		case c.issueCursor == 0:
			c.issueMode = issueModeURL
			c.issueInput.SetValue(c.gitlabURL)
			c.issueInput.Focus()
		case c.issueCursor <= len(c.gitlabProjects):
			c.issueTarget = c.issueCursor - 1
			c.issueMode = issueModeProject
			c.issueInput.SetValue(c.gitlabProjects[c.issueTarget])
			c.issueInput.Focus()
		default:
			c.issueTarget = len(c.gitlabProjects)
			c.issueMode = issueModeProject
			c.issueInput.SetValue("")
			c.issueInput.Focus()
		}
	case "d", "x":
		if c.issueCursor > 0 && c.issueCursor <= len(c.gitlabProjects) {
			i := c.issueCursor - 1
			c.gitlabProjects = append(c.gitlabProjects[:i], c.gitlabProjects[i+1:]...)
			if c.issueCursor > len(c.gitlabProjects)+1 {
				c.issueCursor--
			}
		}
	}
	return c
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
	case secIssues:
		rows, hint = c.renderIssues(t)
	case secBackup:
		rows, hint = c.renderBackup(t)
	default:
		rows, hint = c.renderGeneral(t)
	}

	lines := []string{heading, tabRow, sep, ""}
	lines = append(lines, rows...)
	lines = append(lines, "", hint)
	innerWidth := max(1, width-4)
	for i, line := range lines {
		if lipgloss.Width(line) > innerWidth {
			lines[i] = xansi.Truncate(line, innerWidth, "")
		}
	}
	if maxLines := max(1, height-2); len(lines) > maxLines {
		lines = lines[:maxLines]
	}

	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		Padding(1, 2).
		Render(strings.Join(lines, "\n"))
}

func (c configPane) renderBackup(t Theme) ([]string, string) {
	interval := "manual"
	if minutes := backupIntervals[c.backupInterval]; minutes > 0 {
		interval = fmt.Sprintf("every %dm", minutes)
	}
	destination := c.backupDestination
	if destination == "" {
		destination = "not configured"
	}
	values := []string{
		backupModes[c.backupMode],
		destination,
		interval,
		fmt.Sprintf("%ds", backupTimeouts[c.backupTimeout]),
	}
	labels := []string{"Mode", "Destination", "Interval", "Timeout"}
	rows := make([]string, len(labels))
	for i := range labels {
		prefix := "  "
		valueStyle := lipgloss.NewStyle().Foreground(t.TextSecond)
		if c.backupCursor == i {
			prefix = "▶ "
			valueStyle = valueStyle.Bold(true).Foreground(t.Accent)
		}
		value := valueStyle.Render(values[i])
		if i == 1 && c.backupCursor == i && c.backupEditing {
			value = c.backupInput.View()
		}
		rows[i] = lipgloss.NewStyle().Width(cfgLabelWidth).Foreground(t.TextMuted).Render(prefix+labels[i]) + value
	}
	rows = append(rows, "", lipgloss.NewStyle().Foreground(t.TextDim).Render(
		"Remote uses native Git SSH/HTTPS credentials; path writes an atomic .bundle."))
	return rows, "[↑↓] select   [←→] change   [Enter] edit   [Tab] switch section   [Esc] save & close"
}

func (c configPane) renderIssues(t Theme) ([]string, string) {
	label := lipgloss.NewStyle().Width(cfgLabelWidth)
	var rows []string
	urlSelected := c.issueCursor == 0
	urlLabel := label.Foreground(t.TextMuted).Render("  GitLab URL")
	urlValue := lipgloss.NewStyle().Foreground(t.TextSecond).Render(c.gitlabURL)
	if urlSelected {
		urlLabel = label.Bold(true).Foreground(t.TextPrimary).Render("▶ GitLab URL")
		if c.issueMode == issueModeURL {
			urlValue = c.issueInput.View()
		} else {
			urlValue = lipgloss.NewStyle().Bold(true).Foreground(t.Accent).Render(c.gitlabURL)
		}
	}
	rows = append(rows, urlLabel+urlValue)
	for i, project := range c.gitlabProjects {
		selected := c.issueCursor == i+1
		prefix := "  "
		style := lipgloss.NewStyle().Foreground(t.TextSecond)
		value := style.Render(project)
		if selected {
			prefix = "▶ "
			style = style.Bold(true).Foreground(t.Accent)
			if c.issueMode == issueModeProject {
				value = c.issueInput.View()
			} else {
				value = style.Render(project)
			}
		}
		rows = append(rows, label.Foreground(t.TextMuted).Render(prefix+"Project")+value)
	}
	addSelected := c.issueCursor == len(c.gitlabProjects)+1
	add := "  + Add project"
	if addSelected {
		add = "▶ + Add project"
	}
	if addSelected && c.issueMode == issueModeProject {
		rows = append(rows, label.Render("▶ Project")+c.issueInput.View())
	} else {
		rows = append(rows, lipgloss.NewStyle().Foreground(t.Accent).Render(add))
	}
	return rows, "[↑↓] select   [Enter] edit   [a/n] add project   [d] delete   [Tab] switch section   [Esc] save & close"
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
	if c.cursor == cfgItemTheme && c.values[cfgItemTheme] < len(ThemeChoices) {
		rows = append(rows, "")
		rows = append(rows, renderThemePreview(ThemeChoices[c.values[cfgItemTheme]])...)
	}
	return rows, "[↑↓] select   [←→] preview   [Enter] confirm theme   [Tab] switch section   [Esc] save & close"
}

func renderThemePreview(theme Theme) []string {
	role := func(name string, color lipgloss.Color) string {
		swatch := lipgloss.NewStyle().Background(color).Render("  ")
		return "  " + swatch + " " + lipgloss.NewStyle().Foreground(color).Render(name)
	}
	rows := []string{
		lipgloss.NewStyle().Bold(true).Foreground(theme.Accent).Render("Theme roles · " + themeDisplayName(theme.Name)),
		role("Accent", theme.Accent),
		role("Text primary", theme.TextPrimary),
		role("Text secondary", theme.TextSecond),
		role("Text muted", theme.TextMuted),
		role("Text dim", theme.TextDim),
		role("Border focused", theme.BorderFocus),
		role("Border normal", theme.BorderNormal),
		role("Status bar", theme.StatusBg),
		role("Background", theme.Bg),
		"",
		lipgloss.NewStyle().Background(theme.Accent).Foreground(theme.AccentFg).Render("  ▶ Focused row  ") +
			"  " + lipgloss.NewStyle().Bold(true).Foreground(theme.Accent).Render("# Heading") +
			"  " + lipgloss.NewStyle().Foreground(theme.TextPrimary).Render("Body"),
	}
	return rows
}

func themeDisplayName(name string) string {
	switch name {
	case "solarized":
		return "Solarized Dark"
	case "tokyonight":
		return "Tokyo Night"
	case "solarized-light":
		return "Solarized Light"
	case "catppuccin-mocha":
		return "Catppuccin Mocha"
	case "nord":
		return "Nord"
	case "dracula":
		return "Dracula"
	case "gruvbox":
		return "Gruvbox"
	case "everforest":
		return "Everforest"
	default:
		return name
	}
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
