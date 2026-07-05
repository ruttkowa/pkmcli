package tui

import (
	"sort"
	"strings"

	"pkm/internal/vault"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// slotKind describes the type of argument a command expects.
type slotKind int

const (
	slotNone     slotKind = iota
	slotNote              // note title, fuzzy-matched from vault
	slotTemplate          // note title, filtered to #template-tagged notes only
	slotTitle             // free-text title (new)
	slotQuery             // free-text search query
	slotState             // state choice from vault.AllStates
	slotTheme             // dark|light
	slotArrow             // gateway "→" — separates slots, not user content
	slotProject           // project name, prefix-matched from active projects
	slotVariable          // user-defined {{var}} name, from config
)

// cmdDef is the authoritative grammar entry for one command.
type cmdDef struct {
	name    string
	sig     string     // display signature, e.g. `<note> → <state>`
	desc    string
	example string
	slots   []slotKind // ordered; slotArrow entries act as gateways between content slots
}

var allCommands = []cmdDef{
	{
		name: "new", sig: `"Title"`, desc: "Create a note in Inbox",
		example: `:new "Docker Setup"`,
		slots:   []slotKind{slotTitle},
	},
	{
		name: "new project", sig: "<name>", desc: "Create a new project (max 4)",
		example: `:new project Homelab`,
		slots:   []slotKind{slotProject},
	},
	{
		name: "new template", sig: `"Title"`, desc: "Create a new template note",
		example: `:new template "Meeting Notes"`,
		slots:   []slotKind{slotTitle},
	},
	{
		name: "add project", sig: "<name>", desc: "Assign the open note to a project (moves to Projects)",
		example: `:add project Homelab`,
		slots:   []slotKind{slotProject},
	},
	{
		name: "insert", sig: "<template>", desc: "Insert a template into the current note",
		example: `:insert meeting`,
		slots:   []slotKind{slotTemplate},
	},
	{
		name: "insert var", sig: "<variable>", desc: "Insert a variable's value at the cursor (edit mode only)",
		example: `:insert var author`,
		slots:   []slotKind{slotVariable},
	},
	{
		name: "open", sig: "<note · query · #tag>", desc: "Open by title, search content, or filter by tag",
		example: `:open docker  or  :open #linux`,
		slots:   []slotKind{slotNote},
	},
	{
		name: "move", sig: "<note> → <state>", desc: "Promote or demote a note",
		example: `:move docker → inbox`,
		slots:   []slotKind{slotNote, slotArrow, slotState},
	},
	{
		name: "archive", sig: "<note>", desc: "Move to Archive",
		example: `:archive docker`,
		slots:   []slotKind{slotNote},
	},
	{
		name: "split", sig: "[note]", desc: "Open a new split pane",
		example: `:split  or  :split docker`,
		slots:   []slotKind{slotNote},
	},
	{
		name:    "close",
		sig:     "",
		desc:    "Close the focused pane",
		example: `:close`,
		slots:   []slotKind{},
	},
	{
		name: "theme", sig: "nord · solarized · dracula · gruvbox · tokyonight", desc: "Switch color theme",
		example: `:theme nord`,
		slots:   []slotKind{slotTheme},
	},
	{
		name: "config", sig: "[theme|export|import] ...", desc: "Open config menu, or set/export/import config",
		example: `:config export`,
		slots:   []slotKind{slotNone},
	},
	{
		name:    "help",
		sig:     "",
		desc:    "Show help (keybindings, commands, workflows)",
		example: `:help`,
		slots:   []slotKind{},
	},
	{
		name:    "quit",
		sig:     "",
		desc:    "Save session and quit",
		example: `:quit`,
		slots:   []slotKind{},
	},
	{
		name:    "exit",
		sig:     "",
		desc:    "Save session and quit (alias for :quit)",
		example: `:exit`,
		slots:   []slotKind{},
	},
}

const (
	maxVerbRows    = 8
	maxContextRows = 5
)

// ctxSuggestion is one row in the context-aware (post-verb) dropdown.
type ctxSuggestion struct {
	cmd  string
	desc string
}

type paletteModel struct {
	input        string
	selected     int
	submitted    bool
	cancelled    bool
	notes        []*vault.Note // pre-sorted by Updated desc for note completions
	projectNames []string      // active project names for slotProject autosuggest
	varNames     []string      // configured variable names for slotVariable autosuggest
	verbCtx      verbContext   // biases verb-suggestion order; zero value = declaration order
}

// withVariables sets the variable names available for :insert var autosuggest.
func (p paletteModel) withVariables(names []string) paletteModel {
	p.varNames = names
	return p
}

// verbContext describes why the palette was opened, so the verb list can be
// reordered toward the commands most likely to be used from that situation.
// This is a fixed, hand-authored bias — not a learned/frequency ranking.
type verbContext int

const (
	ctxDefault  verbContext = iota
	ctxNoteOpen             // opened while a note is showing in the main pane
	ctxEditing              // opened via Ctrl+Space from inside the editor (save-and-exit already ran)
)

// verbPriority ranks command names within a context; higher sorts first.
// Commands absent from a context's map keep their declaration-order position.
var verbPriority = map[verbContext]map[string]int{
	ctxNoteOpen: {
		"move":    3,
		"archive": 2,
		"insert":  1,
	},
	ctxEditing: {
		"insert":       5,
		"new template": 3,
		"move":         2,
		"archive":      1,
	},
}

// withContext sets the verb-ranking bias for this palette instance.
func (p paletteModel) withContext(ctx verbContext) paletteModel {
	p.verbCtx = ctx
	return p
}

func newPalette(notes []*vault.Note, projectNames []string) paletteModel {
	return paletteModel{notes: notes, projectNames: projectNames}
}

func newPaletteWithInput(input string, notes []*vault.Note, projectNames []string) paletteModel {
	return paletteModel{input: input, notes: notes, projectNames: projectNames}
}

func (p paletteModel) value() string { return strings.TrimSpace(p.input) }

// ── DSL analysis ─────────────────────────────────────────────────────────────

// isTypingVerb returns true when the input hasn't yet matched a complete command name.
func (p paletteModel) isTypingVerb() bool {
	_, ok := p.currentCmdDef()
	return !ok
}

// inputAfterVerb returns the text after the matched command name and its trailing space.
func (p paletteModel) inputAfterVerb() string {
	trimmed := strings.TrimLeft(p.input, " ")
	def, ok := p.currentCmdDef()
	if !ok {
		return ""
	}
	return trimmed[len(def.name)+1:] // strip "<command> "
}

// currentCmdDef returns the longest command whose name (plus a space) is a prefix of the input.
// Longest-match means "new project " wins over "new " for compound commands.
func (p paletteModel) currentCmdDef() (cmdDef, bool) {
	trimmed := strings.TrimLeft(p.input, " ")
	var best cmdDef
	found := false
	for _, c := range allCommands {
		if strings.HasPrefix(trimmed, c.name+" ") && len(c.name) > len(best.name) {
			best = c
			found = true
		}
	}
	return best, found
}

// activeSlot returns which content slot the user is currently filling.
// slotArrow entries in def.slots act as gateways: if the arrow is present in
// the input we advance past it; if not, we stay at the slot before it.
func (p paletteModel) activeSlot() slotKind {
	def, ok := p.currentCmdDef()
	if !ok || len(def.slots) == 0 {
		return slotNone
	}

	rest := p.inputAfterVerb()
	var current slotKind = slotNone

	for _, s := range def.slots {
		if s == slotArrow {
			crossed := false
			for _, arrow := range []string{"→", "->"} {
				if idx := strings.Index(rest, arrow); idx != -1 {
					rest = strings.TrimLeft(rest[idx+len(arrow):], " ")
					crossed = true
					break
				}
			}
			if !crossed {
				return current // stuck before gateway
			}
			current = slotNone // reset; next regular slot sets it
			continue
		}
		current = s
	}
	return current
}

// currentNoteFragment returns the note-name portion of the input (before any arrow).
func (p paletteModel) currentNoteFragment() string {
	rest := p.inputAfterVerb()
	for _, arrow := range []string{"→", "->"} {
		if idx := strings.Index(rest, arrow); idx != -1 {
			rest = rest[:idx]
		}
	}
	return strings.TrimSpace(rest)
}

// currentStateFragment returns the state text after the arrow in a move command.
func (p paletteModel) currentStateFragment() string {
	rest := p.inputAfterVerb()
	for _, arrow := range []string{"→", "->"} {
		if idx := strings.Index(rest, arrow); idx != -1 {
			return strings.TrimSpace(rest[idx+len(arrow):])
		}
	}
	return ""
}

// ── Suggestions ──────────────────────────────────────────────────────────────

func (p paletteModel) verbSuggestions() []cmdDef {
	trimmed := strings.TrimLeft(p.input, " ")
	var out []cmdDef
	if trimmed == "" {
		out = append(out, allCommands...)
	} else {
		for _, c := range allCommands {
			if strings.HasPrefix(c.name, trimmed) {
				out = append(out, c)
			}
		}
	}

	if prio := verbPriority[p.verbCtx]; len(prio) > 0 {
		sort.SliceStable(out, func(i, j int) bool {
			return prio[out[i].name] > prio[out[j].name]
		})
	}
	return out
}

func (p paletteModel) contextSuggestions() []ctxSuggestion {
	switch p.activeSlot() {
	case slotNote:
		return p.noteSuggestions(p.currentNoteFragment())
	case slotTemplate:
		return p.templateSuggestions(p.currentNoteFragment())
	case slotState:
		return p.stateSuggestions(p.currentStateFragment())
	case slotTheme:
		return p.themeSuggestions(strings.TrimSpace(p.inputAfterVerb()))
	case slotProject:
		return p.projectSuggestions(strings.TrimSpace(p.inputAfterVerb()))
	case slotVariable:
		return p.variableSuggestions(strings.TrimSpace(p.inputAfterVerb()))
	}
	return nil
}

func (p paletteModel) noteSuggestions(fragment string) []ctxSuggestion {
	fl := strings.ToLower(fragment)
	var out []ctxSuggestion
	for _, n := range p.notes {
		if fl == "" || strings.Contains(strings.ToLower(n.Title), fl) {
			out = append(out, ctxSuggestion{cmd: n.Title, desc: string(n.State)})
			if len(out) >= maxContextRows {
				break
			}
		}
	}
	return out
}

func (p paletteModel) templateSuggestions(fragment string) []ctxSuggestion {
	fl := strings.ToLower(fragment)
	var out []ctxSuggestion
	for _, n := range p.notes {
		hasTag := false
		for _, tag := range n.Tags {
			if strings.ToLower(tag) == "template" {
				hasTag = true
				break
			}
		}
		if !hasTag {
			continue
		}
		if fl == "" || strings.Contains(strings.ToLower(n.Title), fl) {
			out = append(out, ctxSuggestion{cmd: n.Title, desc: "template"})
			if len(out) >= maxContextRows {
				break
			}
		}
	}
	return out
}

var stateDescs = map[vault.NoteState]string{
	vault.StateInbox:    "new, unprocessed",
	vault.StateProjects: "active project",
	vault.StateAreas:    "ongoing area",
	vault.StateResearch: "reference material",
	vault.StateArchive:  "inactive / done",
}

func (p paletteModel) stateSuggestions(fragment string) []ctxSuggestion {
	fl := strings.ToLower(fragment)
	var out []ctxSuggestion
	for _, s := range vault.AllStates {
		name := string(s)
		if fl == "" || strings.HasPrefix(name, fl) {
			out = append(out, ctxSuggestion{cmd: name, desc: stateDescs[s]})
		}
	}
	return out
}

func (p paletteModel) themeSuggestions(fragment string) []ctxSuggestion {
	fl := strings.ToLower(fragment)
	descs := []string{"Nord dark theme", "Solarized Dark theme", "Dracula theme", "Gruvbox Dark theme", "Tokyo Night theme"}
	var all []ctxSuggestion
	for i, t := range ThemeChoices {
		all = append(all, ctxSuggestion{cmd: t.Name, desc: descs[i]})
	}
	if fl == "" {
		return all
	}
	var out []ctxSuggestion
	for _, s := range all {
		if strings.HasPrefix(s.cmd, fl) {
			out = append(out, s)
		}
	}
	return out
}

func (p paletteModel) variableSuggestions(fragment string) []ctxSuggestion {
	fl := strings.ToLower(fragment)
	var out []ctxSuggestion
	for _, name := range p.varNames {
		if fl == "" || strings.HasPrefix(strings.ToLower(name), fl) {
			out = append(out, ctxSuggestion{cmd: name, desc: "variable"})
			if len(out) >= maxContextRows {
				break
			}
		}
	}
	return out
}

func (p paletteModel) projectSuggestions(fragment string) []ctxSuggestion {
	fl := strings.ToLower(fragment)
	var out []ctxSuggestion
	for _, name := range p.projectNames {
		if fl == "" || strings.HasPrefix(strings.ToLower(name), fl) {
			out = append(out, ctxSuggestion{cmd: name, desc: "active project"})
			if len(out) >= maxContextRows {
				break
			}
		}
	}
	return out
}

// ── Height ───────────────────────────────────────────────────────────────────

// dropdownHeight returns the exact number of rows renderDropdown will produce.
func (p paletteModel) dropdownHeight() int {
	if p.isTypingVerb() {
		n := len(p.verbSuggestions())
		if n > maxVerbRows {
			n = maxVerbRows
		}
		return n
	}
	_, ok := p.currentCmdDef()
	if !ok {
		return 0
	}
	// Context mode: 1 header + [1 separator + N suggestions] when N > 0.
	sug := p.contextSuggestions()
	n := len(sug)
	if n > maxContextRows {
		n = maxContextRows
	}
	if n == 0 {
		return 1
	}
	return 1 + 1 + n
}

// ── Update ───────────────────────────────────────────────────────────────────

func (p paletteModel) update(msg tea.KeyMsg) (paletteModel, tea.Cmd) {
	switch msg.String() {
	case "enter":
		p.submitted = true

	case "esc":
		p.cancelled = true

	case "ctrl+c":
		p.input = ""
		p.selected = 0

	case "tab", "right":
		p = p.tabComplete()

	case "up", "ctrl+p":
		if n := p.navLen(); n > 0 {
			p.selected = (p.selected - 1 + n) % n
		}

	case "down", "ctrl+n":
		if n := p.navLen(); n > 0 {
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

func (p paletteModel) navLen() int {
	if p.isTypingVerb() {
		n := len(p.verbSuggestions())
		if n > maxVerbRows {
			return maxVerbRows
		}
		return n
	}
	sug := p.contextSuggestions()
	if len(sug) > maxContextRows {
		return maxContextRows
	}
	return len(sug)
}

func (p paletteModel) tabComplete() paletteModel {
	if p.isTypingVerb() {
		verbs := p.verbSuggestions()
		if len(verbs) == 0 {
			return p
		}
		sel := p.selected
		if sel >= len(verbs) {
			sel = 0
		}
		p.input = verbs[sel].name + " "
		p.selected = 0
		return p
	}

	sug := p.contextSuggestions()
	if len(sug) == 0 {
		return p
	}
	sel := p.selected
	if sel >= len(sug) {
		sel = 0
	}
	chosen := sug[sel].cmd

	def, ok := p.currentCmdDef()
	if !ok {
		return p
	}

	switch p.activeSlot() {
	case slotNote, slotTemplate:
		p.input = def.name + " " + chosen
		for _, s := range def.slots {
			if s == slotArrow {
				p.input += " " // space before user types →
				break
			}
		}
	case slotState:
		p.input = def.name + " " + p.currentNoteFragment() + " → " + chosen
	case slotTheme, slotProject, slotVariable:
		p.input = def.name + " " + chosen
	}
	p.selected = 0
	return p
}

// ── Render ───────────────────────────────────────────────────────────────────

func (p paletteModel) renderDropdown(width int) string {
	if p.isTypingVerb() {
		return p.renderVerbDropdown(width)
	}
	return p.renderContextDropdown(width)
}

func (p paletteModel) renderVerbDropdown(width int) string {
	verbs := p.verbSuggestions()
	if len(verbs) == 0 {
		return ""
	}
	limit := len(verbs)
	if limit > maxVerbRows {
		limit = maxVerbRows
	}

	t := activeTheme
	selRow := lipgloss.NewStyle().Background(t.Accent).Foreground(t.AccentFg).Width(width)
	normRow := lipgloss.NewStyle().Background(t.DropdownBg).Foreground(t.TextPrimary).Width(width)
	boldName := lipgloss.NewStyle().Bold(true)
	mutedSig := lipgloss.NewStyle().Foreground(t.TextSecond)
	mutedDesc := lipgloss.NewStyle().Foreground(t.TextDim)

	rows := make([]string, limit)
	for i, c := range verbs[:limit] {
		indicator := "  "
		if i == p.selected {
			indicator = "▶ "
		}
		line := indicator + boldName.Render(c.name)
		if c.sig != "" {
			line += "  " + mutedSig.Render(c.sig)
		}
		line += "   " + mutedDesc.Render(c.desc)
		if i == p.selected {
			rows[i] = selRow.Render(line)
		} else {
			rows[i] = normRow.Render(line)
		}
	}
	return strings.Join(rows, "\n")
}

func (p paletteModel) renderContextDropdown(width int) string {
	def, ok := p.currentCmdDef()
	if !ok {
		return ""
	}

	t := activeTheme
	bg := t.DropdownBg

	// Header: "▌ verb sig" on the left, "e.g. :example" on the right.
	accentBold := lipgloss.NewStyle().Bold(true).Foreground(t.Accent).Background(bg)
	dimSig := lipgloss.NewStyle().Foreground(t.TextSecond).Background(bg)
	dimEg := lipgloss.NewStyle().Foreground(t.TextDim).Background(bg)

	line := "▌ " + accentBold.Render(def.name)
	if def.sig != "" {
		line += " " + dimSig.Render(def.sig)
	}
	if def.example != "" {
		line += "   " + dimEg.Render("e.g. "+def.example)
	}
	header := lipgloss.NewStyle().Background(bg).Width(width).Render(line)

	sug := p.contextSuggestions()
	if len(sug) > maxContextRows {
		sug = sug[:maxContextRows]
	}
	if len(sug) == 0 {
		return header
	}

	sep := lipgloss.NewStyle().Background(bg).Foreground(t.TextDim).Width(width).
		Render(strings.Repeat("─", width))

	selRow := lipgloss.NewStyle().Background(t.Accent).Foreground(t.AccentFg).Width(width)
	normRow := lipgloss.NewStyle().Background(bg).Foreground(t.TextPrimary).Width(width)
	dimDesc := lipgloss.NewStyle().Foreground(t.TextDim)

	rows := []string{header, sep}
	for i, s := range sug {
		indicator := "  "
		if i == p.selected {
			indicator = "▶ "
		}
		line := indicator + s.cmd
		if s.desc != "" {
			line += "   " + dimDesc.Render(s.desc)
		}
		if i == p.selected {
			rows = append(rows, selRow.Render(line))
		} else {
			rows = append(rows, normRow.Render(line))
		}
	}
	return strings.Join(rows, "\n")
}

func (p paletteModel) renderInputLine(width int) string {
	t := activeTheme
	bg := t.DropdownBg
	prefix := lipgloss.NewStyle().Background(bg).Foreground(t.Cursor).Bold(true).Render(":")
	text := lipgloss.NewStyle().Background(bg).Foreground(t.AccentFg).Render(" " + p.input + "█")
	return lipgloss.NewStyle().Background(bg).Width(width).Render(prefix + text)
}
