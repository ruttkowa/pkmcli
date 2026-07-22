package tui

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"pkm/internal/vault"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
)

const (
	editHeaderRows = 5 // title + state + tags + project + separator
	editFooterRows = 1 // word count bar
	editLabelWidth = 10

	linkSuggestMax = 3 // max suggestion rows shown
)

var numberedListRe = regexp.MustCompile(`^(\d+)\. `)
var headingLineRe = regexp.MustCompile(`^#{1,6}( |$)`)

const headingGutterMarker = "▎"

type focusedField int

const (
	fldState   focusedField = iota
	fldTags
	fldProject
	fldBody
	fldCount
)

type editPane struct {
	note      *vault.Note
	ta        textarea.Model
	tagsInput textinput.Model
	projInput textinput.Model
	stateIdx  int
	focused   focusedField
	wordCount int
	saved     bool
	cancelled bool

	notes         []*vault.Note // for link autocomplete
	projectNames  []string      // active project names, for the Project field autosuggest
	contentHeight int
	saveKey       string // configured key that commits the draft (default "ctrl+s")
	// link suggestion state
	linkSuggestActive bool
	linkSuggestFrag   string
	linkSuggestSel    int
	linkSuggestList   []string
	// project field suggestion state
	projSuggestActive bool
	projSuggestSel    int
	projSuggestList   []string

	// Line-wise operations (#10): Ctrl+L is a leader — the next key (y/d/p)
	// runs the operation; anything else aborts, discarding that key rather
	// than inserting it. lineRegister lives for this editor session only,
	// reset (with the rest of editPane) when the pane closes.
	lineOpPending bool
	lineRegister  string
	lineOpStatus  string // one-shot feedback ("line yanked" etc.); model.go copies it to statusMsg and clears it
}

func newEditPane(n *vault.Note, width, height int, notes []*vault.Note, projectNames []string, lineNumbers bool, saveKey string, startLine int) (editPane, tea.Cmd) {
	si := 0
	for i, s := range vault.AllStates {
		if s == n.State {
			si = i
			break
		}
	}

	inputW := max(1, width-editLabelWidth)

	tags := textinput.New()
	tags.Prompt = ""
	tags.CharLimit = 0
	tags.Placeholder = "comma-separated"
	tags.Width = inputW
	tags.SetValue(strings.Join(n.Tags, ", "))
	applyTextInputTheme(&tags)

	proj := textinput.New()
	proj.Prompt = ""
	proj.CharLimit = 0
	proj.Placeholder = "project name"
	proj.Width = inputW
	proj.SetValue(n.Project)
	applyTextInputTheme(&proj)

	ta := textarea.New()
	ta.Prompt = " "
	ta.ShowLineNumbers = lineNumbers
	ta.CharLimit = 0
	bodyH := calcBodyHeight(height)
	ta.SetWidth(width)
	ta.SetHeight(bodyH)
	applyEditTheme(&ta)
	ta.SetValue(n.Body)
	// Focus must happen before any Update() call below — textarea.Update
	// no-ops entirely while unfocused (m.focus check), so the Ctrl+Home
	// reset silently did nothing when called beforehand (a latent bug
	// predating #22: SetValue leaves the cursor at the document's end, and
	// with Focus called after Update, the editor always opened there).
	focusCmd := ta.Focus()
	ta, _ = ta.Update(tea.KeyMsg{Type: tea.KeyCtrlHome})
	// #22: resume editing where the viewer's cursor was, not always at the
	// top. CursorDown moves by *display* row, not logical line, so a
	// word-wrapped line takes more than one call to cross — loop on Line()
	// reaching the target rather than counting calls. Clamping startLine
	// into range first guarantees CursorDown (a no-op past the last line)
	// can always reach it, so this can't spin forever.
	if startLine >= ta.LineCount() {
		startLine = ta.LineCount() - 1
	}
	if startLine < 0 {
		startLine = 0
	}
	for ta.Line() < startLine {
		ta.CursorDown()
	}
	ta.SetCursor(0)

	return editPane{
		note:          n,
		ta:            ta,
		tagsInput:     tags,
		projInput:     proj,
		stateIdx:      si,
		focused:       fldBody,
		wordCount:     countWords(n.Body),
		notes:         notes,
		projectNames:  projectNames,
		contentHeight: height,
		saveKey:       saveKey,
	}, focusCmd
}

// dirty reports whether the draft differs from the last-saved note, across
// every editable field, not just the body.
func (e editPane) dirty() bool {
	if e.ta.Value() != e.note.Body {
		return true
	}
	if vault.AllStates[e.stateIdx] != e.note.State {
		return true
	}
	if strings.TrimSpace(e.projInput.Value()) != e.note.Project {
		return true
	}
	tags := parseTags(e.tagsInput.Value())
	if len(tags) != len(e.note.Tags) {
		return true
	}
	for i, t := range tags {
		if t != e.note.Tags[i] {
			return true
		}
	}
	return false
}

// footerText builds the editor's status line, degrading gracefully when the
// pane is too narrow for the full text — a hard-wrapped footer would push
// the pane's rendered height past its allotted box (see calcBodyHeight,
// which reserves exactly editFooterRows=1 for this line), desyncing the
// border. Each stage drops the least essential segment first; the final
// xansi.Truncate is a hard safety net for pathologically narrow panes.
func (e editPane) footerText(width, totalLines int) string {
	saved := e.note.Updated.Format("15:04:05")
	dirty := e.dirty()

	build := func(unsaved, saveLabel, counts, hint string) string {
		parts := make([]string, 0, 4)
		if unsaved != "" {
			parts = append(parts, unsaved)
		}
		if saveLabel != "" {
			parts = append(parts, saveLabel)
		}
		if counts != "" {
			parts = append(parts, counts)
		}
		if hint != "" {
			parts = append(parts, hint)
		}
		return "  " + strings.Join(parts, "   ")
	}

	unsavedFull := ""
	unsavedShort := ""
	if dirty {
		unsavedFull = lipgloss.NewStyle().Foreground(activeTheme.Cursor).Bold(true).Render("●  Unsaved changes")
		unsavedShort = lipgloss.NewStyle().Foreground(activeTheme.Cursor).Bold(true).Render("●")
	}
	saveLabel := "Last saved: " + saved
	counts := fmt.Sprintf("Words: %d  Lines: %d", e.wordCount, totalLines)
	hint := "Tab: cycle fields"
	if e.lineOpPending {
		hint = "line: y yank · d delete · p paste"
	}

	candidates := []string{
		build(unsavedFull, saveLabel, counts, hint),
		build(unsavedFull, saveLabel, counts, ""),
		build(unsavedShort, saveLabel, counts, ""),
		build(unsavedShort, saveLabel, "", ""),
	}
	for _, c := range candidates {
		if lipgloss.Width(c) <= width {
			return c
		}
	}
	return xansi.Truncate(candidates[len(candidates)-1], width, "")
}

// bodyHeight returns the correct textarea height accounting for active suggestions.
func (e editPane) bodyHeight() int {
	h := calcBodyHeight(e.contentHeight)
	if e.linkSuggestActive {
		n := len(e.linkSuggestList)
		if n > linkSuggestMax {
			n = linkSuggestMax
		}
		h -= n
	}
	if e.projSuggestActive {
		n := len(e.projSuggestList)
		if n > linkSuggestMax {
			n = linkSuggestMax
		}
		h -= n
	}
	if h < 1 {
		h = 1
	}
	return h
}

func calcBodyHeight(totalHeight int) int {
	h := totalHeight - editHeaderRows - editFooterRows
	if h < 1 {
		h = 1
	}
	return h
}

func applyTextInputTheme(ti *textinput.Model) {
	bg := activeTheme.Bg
	ti.TextStyle = lipgloss.NewStyle().Foreground(activeTheme.TextPrimary).Background(bg)
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(activeTheme.TextDim).Background(bg)
	ti.PromptStyle = lipgloss.NewStyle().Background(bg)
	ti.Cursor.Style = lipgloss.NewStyle().Foreground(activeTheme.Cursor).Background(bg)
}

func applyEditTheme(ta *textarea.Model) {
	bg := activeTheme.Bg
	ta.FocusedStyle = textarea.Style{
		Base:             lipgloss.NewStyle().Background(bg),
		CursorLine:       lipgloss.NewStyle().Background(activeTheme.BlurredBg),
		CursorLineNumber: lipgloss.NewStyle().Foreground(activeTheme.TextDim).Background(bg),
		EndOfBuffer:      lipgloss.NewStyle().Foreground(activeTheme.TextDim).Background(bg),
		LineNumber:       lipgloss.NewStyle().Foreground(activeTheme.TextDim).Background(bg),
		Placeholder:      lipgloss.NewStyle().Foreground(activeTheme.TextMuted).Background(bg),
		// Accent so the heading gutter marker (see headingPromptFunc) pops;
		// harmless for the blank-space prompt used on non-heading lines.
		Prompt: lipgloss.NewStyle().Foreground(activeTheme.Accent).Bold(true).Background(bg),
		Text:   lipgloss.NewStyle().Foreground(activeTheme.TextPrimary).Background(bg),
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

// headingPromptFunc marks heading lines with a gutter glyph instead of the
// default blank prompt — the closest we can get to "on the fly" heading
// emphasis without forking bubbles/textarea, which only exposes one style
// per row (cursor-line vs. text) and no per-content hook. Row-count math
// approximates the component's own word-wrap (exact wrap breakpoints don't
// matter here, only which display row each raw line starts on).
func headingPromptFunc(value string, width int) func(int) string {
	lines := strings.Split(value, "\n")
	return func(displayLine int) string {
		d := 0
		for _, line := range lines {
			rows := approxWrapRows(line, width)
			if displayLine < d+rows {
				if displayLine == d && headingLineRe.MatchString(line) {
					return headingGutterMarker
				}
				return " "
			}
			d += rows
		}
		return " "
	}
}

func approxWrapRows(line string, width int) int {
	if width <= 0 {
		return 1
	}
	w := lipgloss.Width(line)
	if w == 0 {
		return 1
	}
	return (w + width - 1) / width
}

func (e editPane) update(msg tea.Msg) (editPane, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		// Link suggestion mode intercepts keys before the global switch.
		if e.linkSuggestActive && e.focused == fldBody {
			switch km.String() {
			case "tab", "enter":
				return e.completeLinkSuggestion()
			case "up", "ctrl+p":
				if n := len(e.linkSuggestList); n > 0 {
					e.linkSuggestSel = (e.linkSuggestSel - 1 + n) % n
				}
				return e, nil
			case "down", "ctrl+n":
				if n := len(e.linkSuggestList); n > 0 {
					e.linkSuggestSel = (e.linkSuggestSel + 1) % n
				}
				return e, nil
			case "esc":
				// Dismiss suggestions without cancelling the editor.
				e.linkSuggestActive = false
				e.linkSuggestList = nil
				e.ta.SetHeight(calcBodyHeight(e.contentHeight))
				return e, nil
			}
		}

		// Project suggestion dropdown intercepts keys before the global
		// switch, same pattern as the link suggestion block above.
		if e.projSuggestActive && e.focused == fldProject {
			switch km.String() {
			case "tab", "enter":
				return e.completeProjectSuggestion()
			case "up", "ctrl+p":
				if n := len(e.projSuggestList); n > 0 {
					e.projSuggestSel = (e.projSuggestSel - 1 + n) % n
				}
				return e, nil
			case "down", "ctrl+n":
				if n := len(e.projSuggestList); n > 0 {
					e.projSuggestSel = (e.projSuggestSel + 1) % n
				}
				return e, nil
			case "esc":
				// Dismiss suggestions without cancelling the editor.
				e.projSuggestActive = false
				e.projSuggestList = nil
				e.ta.SetHeight(calcBodyHeight(e.contentHeight))
				return e, nil
			}
		}

		// #10: line-wise yank/delete/paste, Ctrl+L leader chord. Must
		// intercept before the global switch below, same reason as the two
		// suggestion blocks above — that switch's own "esc" case (cancels
		// the whole editor) and saveKey case would otherwise steal the
		// chord's next key before this ever saw it, e.g. turning "Ctrl+L
		// then Esc" (abort the chord) into "cancel the editor".
		if e.lineOpPending && e.focused == fldBody {
			e.lineOpPending = false
			switch km.String() {
			case "y":
				e = e.yankCurrentLine()
				e.lineOpStatus = "line yanked"
			case "d":
				e = e.deleteCurrentLine()
				e.lineOpStatus = "line deleted"
			case "p":
				if e.lineRegister == "" {
					e.lineOpStatus = "nothing to paste"
				} else {
					e = e.pasteLineBelow()
					e.lineOpStatus = "line pasted"
				}
			}
			return e, nil
		}
		if km.String() == "ctrl+l" && e.focused == fldBody {
			e.lineOpPending = true
			return e, nil
		}

		switch km.String() {
		case e.saveKey:
			e.saved = true
			return e, nil
		case "esc":
			e.cancelled = true
			return e, nil
		case "tab":
			return e.cycleField(1)
		case "shift+tab":
			return e.cycleField(-1)
		}

		switch e.focused {
		case fldState:
			return e.updateState(km), nil
		case fldTags:
			var cmd tea.Cmd
			e.tagsInput, cmd = e.tagsInput.Update(km)
			return e, cmd
		case fldProject:
			var cmd tea.Cmd
			e.projInput, cmd = e.projInput.Update(km)
			e = e.refreshProjectSuggest()
			return e, cmd
		case fldBody:
			return e.updateBody(km)
		}
	}

	// Non-key messages (cursor blink ticks) → active field only.
	switch e.focused {
	case fldTags:
		var cmd tea.Cmd
		e.tagsInput, cmd = e.tagsInput.Update(msg)
		return e, cmd
	case fldProject:
		var cmd tea.Cmd
		e.projInput, cmd = e.projInput.Update(msg)
		return e, cmd
	default:
		var cmd tea.Cmd
		e.ta, cmd = e.ta.Update(msg)
		return e, cmd
	}
}

func (e editPane) updateState(km tea.KeyMsg) editPane {
	n := len(vault.AllStates)
	switch km.String() {
	case "left", "h":
		e.stateIdx = (e.stateIdx + n - 1) % n
	case "right", "l", "enter":
		e.stateIdx = (e.stateIdx + 1) % n
	}
	return e
}

func (e editPane) updateBody(km tea.KeyMsg) (editPane, tea.Cmd) {
	after := charAfterCursor(e.ta)

	switch km.String() {
	case "[":
		e.ta.InsertRune('[')
		e.ta.InsertRune(']')
		e.ta, _ = e.ta.Update(tea.KeyMsg{Type: tea.KeyLeft})
		e.wordCount = countWords(e.ta.Value())
		e = e.refreshLinkSuggest()
		return e, nil

	case "]":
		if after == ']' {
			e.ta, _ = e.ta.Update(tea.KeyMsg{Type: tea.KeyRight})
		} else {
			e.ta.InsertRune(']')
		}
		e = e.refreshLinkSuggest()
		return e, nil

	case "(":
		e.ta.InsertRune('(')
		e.ta.InsertRune(')')
		e.ta, _ = e.ta.Update(tea.KeyMsg{Type: tea.KeyLeft})
		return e, nil

	case ")":
		if after == ')' {
			e.ta, _ = e.ta.Update(tea.KeyMsg{Type: tea.KeyRight})
		} else {
			e.ta.InsertRune(')')
		}
		return e, nil

	case "`":
		before := textBeforeCursor(e.ta)
		switch {
		case strings.HasSuffix(before, "``") && after != '`':
			// Completing a fence: "``" + this "`" == "```". Expand into a
			// full fenced block instead of autoclosing a 4th backtick.
			e.ta.InsertString("`\n\n```")
			e.ta, _ = e.ta.Update(tea.KeyMsg{Type: tea.KeyUp})
		case after == '`':
			e.ta, _ = e.ta.Update(tea.KeyMsg{Type: tea.KeyRight})
		default:
			e.ta.InsertRune('`')
			e.ta.InsertRune('`')
			e.ta, _ = e.ta.Update(tea.KeyMsg{Type: tea.KeyLeft})
		}
		return e, nil

	case "enter":
		e = e.handleEnter()
		e.wordCount = countWords(e.ta.Value())
		return e, nil

	case "backspace":
		// #21: the two halves of an auto-paired ( ), [ ], or `` are inserted
		// together but bubbles/textarea's own backspace only ever deletes one
		// rune, leaving an orphaned closer behind (and, on the next opener
		// keystroke, a stray extra pair). If the cursor sits exactly between
		// a matching pair with nothing in between, delete both; otherwise a
		// normal single-rune backspace.
		var cmd tea.Cmd
		if want, ok := autoPairs[charBeforeCursor(e.ta)]; ok && after == want {
			e.ta, _ = e.ta.Update(tea.KeyMsg{Type: tea.KeyDelete})
			e.ta, cmd = e.ta.Update(tea.KeyMsg{Type: tea.KeyBackspace})
		} else {
			e.ta, cmd = e.ta.Update(km)
		}
		e.wordCount = countWords(e.ta.Value())
		e = e.refreshLinkSuggest()
		return e, cmd
	}

	var cmd tea.Cmd
	e.ta, cmd = e.ta.Update(km)
	e.wordCount = countWords(e.ta.Value())
	e = e.refreshLinkSuggest()
	return e, cmd
}

// yankCurrentLine copies the line the cursor is on into lineRegister. No
// buffer mutation, so the cursor is untouched.
func (e editPane) yankCurrentLine() editPane {
	lines := strings.Split(e.ta.Value(), "\n")
	idx := e.ta.Line()
	if idx < 0 || idx >= len(lines) {
		return e
	}
	e.lineRegister = lines[idx]
	return e
}

// deleteCurrentLine cuts the line the cursor is on into lineRegister and
// removes it from the buffer, landing the cursor at the start of whatever
// line now occupies that index (clamped if the last line was deleted).
func (e editPane) deleteCurrentLine() editPane {
	lines := strings.Split(e.ta.Value(), "\n")
	idx := e.ta.Line()
	if idx < 0 || idx >= len(lines) {
		return e
	}
	e.lineRegister = lines[idx]
	lines = append(lines[:idx], lines[idx+1:]...)
	if len(lines) == 0 {
		lines = []string{""} // never leave the textarea with zero lines
	}
	e.ta.SetValue(strings.Join(lines, "\n"))
	e = e.setCursorLine(idx, 0)
	e.wordCount = countWords(e.ta.Value())
	return e
}

// pasteLineBelow inserts lineRegister as a new line directly below the
// cursor line (vim's "p"), landing the cursor on it.
func (e editPane) pasteLineBelow() editPane {
	lines := strings.Split(e.ta.Value(), "\n")
	idx := e.ta.Line()
	if idx < 0 {
		idx = 0
	}
	insertAt := idx + 1
	if insertAt > len(lines) {
		insertAt = len(lines)
	}
	newLines := make([]string, 0, len(lines)+1)
	newLines = append(newLines, lines[:insertAt]...)
	newLines = append(newLines, e.lineRegister)
	newLines = append(newLines, lines[insertAt:]...)
	e.ta.SetValue(strings.Join(newLines, "\n"))
	e = e.setCursorLine(insertAt, 0)
	e.wordCount = countWords(e.ta.Value())
	return e
}

// setCursorLine repositions e.ta's cursor to a specific logical line and
// column. SetValue always leaves the cursor at the document's end (it's
// Reset+InsertString under the hood), so every line op that mutates the
// buffer must restore a real position afterward — clamped into range so
// deleting the last line can't leave it out of bounds. Same CursorDown-
// loops-to-target-line technique as #22's newEditPane (CursorDown moves by
// *display* row, not logical line, so a wrapped line takes more than one
// call to cross).
func (e editPane) setCursorLine(line, col int) editPane {
	if line >= e.ta.LineCount() {
		line = e.ta.LineCount() - 1
	}
	if line < 0 {
		line = 0
	}
	e.ta, _ = e.ta.Update(tea.KeyMsg{Type: tea.KeyCtrlHome})
	for e.ta.Line() < line {
		e.ta.CursorDown()
	}
	e.ta.SetCursor(col)
	return e
}

// autoPairs maps each auto-closed opener to its closer — the same set
// updateBody auto-closes on ("(", "[", "`") — used by the backspace handler
// above to detect an empty pair.
var autoPairs = map[rune]rune{'(': ')', '[': ']', '`': '`'}

// detectLinkFragment returns the typed fragment inside an active [[...]] at the cursor.
func (e editPane) detectLinkFragment() (string, bool) {
	lineIdx := e.ta.Line()
	lines := strings.Split(e.ta.Value(), "\n")
	if lineIdx >= len(lines) {
		return "", false
	}
	runes := []rune(lines[lineIdx])
	col := e.ta.LineInfo().CharOffset
	if col > len(runes) {
		col = len(runes)
	}
	before := string(runes[:col])
	// Find the last [[ without a closing ]] before the cursor.
	idx := strings.LastIndex(before, "[[")
	if idx == -1 {
		return "", false
	}
	frag := before[idx+2:]
	if strings.Contains(frag, "]]") {
		return "", false
	}
	// Cursor must be immediately before ]] to confirm we're inside the link.
	after := string(runes[col:])
	if !strings.HasPrefix(after, "]]") {
		return "", false
	}
	return frag, true
}

// refreshLinkSuggest updates the suggestion state based on current cursor position.
func (e editPane) refreshLinkSuggest() editPane {
	frag, active := e.detectLinkFragment()
	prevLen := len(e.linkSuggestList)

	if !active {
		if e.linkSuggestActive {
			e.linkSuggestActive = false
			e.linkSuggestList = nil
			e.ta.SetHeight(calcBodyHeight(e.contentHeight))
		}
		return e
	}

	e.linkSuggestActive = true
	if frag != e.linkSuggestFrag {
		e.linkSuggestFrag = frag
		e.linkSuggestSel = 0
		e.linkSuggestList = e.filterNotes(frag)
	}

	newLen := len(e.linkSuggestList)
	if newLen > linkSuggestMax {
		newLen = linkSuggestMax
	}
	if prevLen > linkSuggestMax {
		prevLen = linkSuggestMax
	}
	if newLen != prevLen {
		e.ta.SetHeight(e.bodyHeight())
	}
	return e
}

func (e editPane) filterNotes(fragment string) []string {
	fl := strings.ToLower(fragment)
	var out []string
	for _, n := range e.notes {
		if fl == "" || strings.Contains(strings.ToLower(n.Title), fl) {
			out = append(out, n.Title)
			if len(out) >= linkSuggestMax {
				break
			}
		}
	}
	return out
}

// completeLinkSuggestion replaces the typed fragment with the selected note title.
func (e editPane) completeLinkSuggestion() (editPane, tea.Cmd) {
	if !e.linkSuggestActive || len(e.linkSuggestList) == 0 {
		return e, nil
	}
	sel := e.linkSuggestSel
	if sel >= len(e.linkSuggestList) {
		sel = 0
	}
	title := e.linkSuggestList[sel]

	// Delete the fragment characters (cursor is between [[ and ]]).
	for range []rune(e.linkSuggestFrag) {
		e.ta, _ = e.ta.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	}
	for _, r := range title {
		e.ta.InsertRune(r)
	}

	e.linkSuggestActive = false
	e.linkSuggestList = nil
	e.linkSuggestFrag = ""
	e.linkSuggestSel = 0
	e.ta.SetHeight(calcBodyHeight(e.contentHeight))
	e.wordCount = countWords(e.ta.Value())
	return e, nil
}

func (e editPane) handleEnter() editPane {
	lineIdx := e.ta.Line()
	lines := strings.Split(e.ta.Value(), "\n")
	var currentLine string
	if lineIdx < len(lines) {
		currentLine = lines[lineIdx]
	}

	// Enter on a list/task marker with no text typed after it breaks out of
	// the list instead of continuing it: clear the empty marker and drop to
	// a plain blank line. This is the terminal-safe stand-in for the
	// originally requested Shift+Enter break-out — this Bubble Tea setup
	// cannot distinguish Shift+Enter from plain Enter (both arrive as the
	// same KeyMsg), so there is no separate binding to hang that behavior
	// off of.
	breakOutOfEmptyMarker := func() editPane {
		for range []rune(currentLine) {
			e.ta, _ = e.ta.Update(tea.KeyMsg{Type: tea.KeyBackspace})
		}
		return e
	}

	switch {
	case currentLine == "- [ ] ", currentLine == "- [x] ", currentLine == "- ", currentLine == "* ":
		return breakOutOfEmptyMarker()
	case strings.HasPrefix(currentLine, "- [x] "), strings.HasPrefix(currentLine, "- [ ] "):
		e.ta.InsertString("\n- [ ] ")
	case strings.HasPrefix(currentLine, "- "):
		e.ta.InsertString("\n- ")
	case strings.HasPrefix(currentLine, "* "):
		e.ta.InsertString("\n* ")
	default:
		if m := numberedListRe.FindStringSubmatch(currentLine); m != nil {
			if currentLine == m[0] {
				return breakOutOfEmptyMarker()
			}
			n, _ := strconv.Atoi(m[1])
			e.ta.InsertString(fmt.Sprintf("\n%d. ", n+1))
		} else {
			e.ta.InsertString("\n")
		}
	}
	return e
}

func (e editPane) cycleField(dir int) (editPane, tea.Cmd) {
	switch e.focused {
	case fldTags:
		e.tagsInput.Blur()
	case fldProject:
		e.projInput.Blur()
		e.projSuggestActive = false
		e.projSuggestList = nil
		e.ta.SetHeight(calcBodyHeight(e.contentHeight))
	case fldBody:
		e.ta.Blur()
	}

	e.focused = focusedField((int(e.focused) + int(fldCount) + dir) % int(fldCount))

	switch e.focused {
	case fldTags:
		cmd := e.tagsInput.Focus()
		return e, cmd
	case fldProject:
		cmd := e.projInput.Focus()
		e = e.refreshProjectSuggest()
		return e, cmd
	case fldBody:
		cmd := e.ta.Focus()
		return e, cmd
	}
	return e, nil // fldState has no focus command
}

// refreshProjectSuggest recomputes the Project field's suggestion dropdown
// from the current input value, mirroring the palette's slotProject
// autosuggest (case-insensitive prefix match over active project names).
func (e editPane) refreshProjectSuggest() editPane {
	prevLen := len(e.projSuggestList)
	if prevLen > linkSuggestMax {
		prevLen = linkSuggestMax
	}

	e.projSuggestList = e.filterProjects(strings.TrimSpace(e.projInput.Value()))
	e.projSuggestActive = len(e.projSuggestList) > 0
	if e.projSuggestSel >= len(e.projSuggestList) {
		e.projSuggestSel = 0
	}

	newLen := len(e.projSuggestList)
	if newLen > linkSuggestMax {
		newLen = linkSuggestMax
	}
	if newLen != prevLen {
		e.ta.SetHeight(e.bodyHeight())
	}
	return e
}

func (e editPane) filterProjects(fragment string) []string {
	fl := strings.ToLower(fragment)
	var out []string
	for _, name := range e.projectNames {
		if fl == "" || strings.HasPrefix(strings.ToLower(name), fl) {
			out = append(out, name)
			if len(out) >= linkSuggestMax {
				break
			}
		}
	}
	return out
}

// completeProjectSuggestion replaces the Project field's value with the
// selected suggestion.
func (e editPane) completeProjectSuggestion() (editPane, tea.Cmd) {
	if !e.projSuggestActive || len(e.projSuggestList) == 0 {
		return e, nil
	}
	sel := e.projSuggestSel
	if sel >= len(e.projSuggestList) {
		sel = 0
	}
	e.projInput.SetValue(e.projSuggestList[sel])
	e.projInput.CursorEnd()

	e.projSuggestActive = false
	e.projSuggestList = nil
	e.projSuggestSel = 0
	e.ta.SetHeight(calcBodyHeight(e.contentHeight))
	return e, nil
}

func (e editPane) render(width, height int) string {
	e.ta.SetPromptFunc(1, headingPromptFunc(e.ta.Value(), e.ta.Width()))

	bg := activeTheme.Bg
	labelStyle := lipgloss.NewStyle().Foreground(activeTheme.TextMuted).Background(bg)
	activeLabelStyle := lipgloss.NewStyle().Foreground(activeTheme.Accent).Background(bg)
	row := lipgloss.NewStyle().Width(width).Background(bg)

	lbl := func(field focusedField, label string) string {
		if e.focused == field {
			return activeLabelStyle.Render(label)
		}
		return labelStyle.Render(label)
	}

	titleRow := row.Render(
		labelStyle.Render(" Title:   ") +
			lipgloss.NewStyle().Foreground(activeTheme.TextPrimary).Bold(true).Background(bg).Render(e.note.Title),
	)
	stateRow := row.Render(lbl(fldState, " State:   ") + renderStateSelector(vault.AllStates[e.stateIdx], e.focused == fldState))
	tagsRow := row.Render(lbl(fldTags, " Tags:    ") + e.tagsInput.View())
	projRow := row.Render(lbl(fldProject, " Project: ") + e.projInput.View())
	sep := lipgloss.NewStyle().Foreground(activeTheme.BorderNormal).Background(bg).Render(strings.Repeat("─", width))

	totalLines := strings.Count(e.ta.Value(), "\n") + 1
	footer := lipgloss.NewStyle().
		Foreground(activeTheme.TextDim).
		Background(activeTheme.StatusBg).
		Width(width).
		Render(e.footerText(width, totalLines))

	parts := []string{titleRow, stateRow, tagsRow, projRow}

	// Show project suggestions directly under the Project field.
	if e.projSuggestActive && len(e.projSuggestList) > 0 {
		t := activeTheme
		limit := len(e.projSuggestList)
		if limit > linkSuggestMax {
			limit = linkSuggestMax
		}
		for i := 0; i < limit; i++ {
			sel := i == e.projSuggestSel
			sty := lipgloss.NewStyle().Width(width).Background(t.DropdownBg).Foreground(t.TextPrimary)
			indicator := "  "
			if sel {
				sty = sty.Background(t.Accent).Foreground(t.AccentFg)
				indicator = "▶ "
			}
			parts = append(parts, sty.Render(indicator+e.projSuggestList[i]))
		}
	}

	parts = append(parts, sep, e.ta.View())

	// Show link suggestions between body and footer.
	if e.linkSuggestActive && len(e.linkSuggestList) > 0 {
		t := activeTheme
		limit := len(e.linkSuggestList)
		if limit > linkSuggestMax {
			limit = linkSuggestMax
		}
		for i := 0; i < limit; i++ {
			sel := i == e.linkSuggestSel
			sty := lipgloss.NewStyle().Width(width).Background(t.DropdownBg).Foreground(t.TextPrimary)
			indicator := "  "
			if sel {
				sty = sty.Background(t.Accent).Foreground(t.AccentFg)
				indicator = "▶ "
			}
			parts = append(parts, sty.Render(indicator+"[["+e.linkSuggestList[i]+"]]"))
		}
	}

	parts = append(parts, footer)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func renderStateSelector(state vault.NoteState, focused bool) string {
	bg := activeTheme.Bg
	s := string(state)
	if !focused {
		return lipgloss.NewStyle().Foreground(activeTheme.TextSecond).Background(bg).Render(s)
	}
	arrow := lipgloss.NewStyle().Foreground(activeTheme.Accent).Background(bg).Render
	val := lipgloss.NewStyle().Foreground(activeTheme.AccentFg).Background(activeTheme.Accent).Padding(0, 1).Render(s)
	return arrow("◀ ") + val + arrow(" ▶")
}

func charAfterCursor(ta textarea.Model) rune {
	lineIdx := ta.Line()
	lines := strings.Split(ta.Value(), "\n")
	if lineIdx >= len(lines) {
		return 0
	}
	runes := []rune(lines[lineIdx])
	col := ta.LineInfo().CharOffset
	if col >= len(runes) {
		return 0
	}
	return runes[col]
}

// textBeforeCursor returns the current line's text up to the cursor column.
func textBeforeCursor(ta textarea.Model) string {
	lineIdx := ta.Line()
	lines := strings.Split(ta.Value(), "\n")
	if lineIdx >= len(lines) {
		return ""
	}
	runes := []rune(lines[lineIdx])
	col := ta.LineInfo().CharOffset
	if col > len(runes) {
		col = len(runes)
	}
	return string(runes[:col])
}

// charBeforeCursor returns the rune immediately before the cursor on the
// current line, or 0 if the cursor is at the start of the line. Derived from
// textBeforeCursor rather than a second line/column traversal.
func charBeforeCursor(ta textarea.Model) rune {
	before := []rune(textBeforeCursor(ta))
	if len(before) == 0 {
		return 0
	}
	return before[len(before)-1]
}

func countWords(s string) int {
	return len(strings.Fields(s))
}

// parseTags splits a comma-separated tag string into a slice, stripping whitespace and leading #.
func parseTags(s string) []string {
	var tags []string
	for _, p := range strings.Split(s, ",") {
		t := strings.TrimSpace(p)
		t = strings.TrimPrefix(t, "#")
		t = strings.TrimSpace(t)
		if t != "" {
			tags = append(tags, t)
		}
	}
	return tags
}
