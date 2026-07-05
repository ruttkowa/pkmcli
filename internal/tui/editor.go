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
)

const (
	editHeaderRows = 5 // title + state + tags + project + separator
	editFooterRows = 1 // word count bar
	editLabelWidth = 10

	linkSuggestMax = 3 // max suggestion rows shown
)

var numberedListRe = regexp.MustCompile(`^(\d+)\. `)

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
	contentHeight int
	saveKey       string // configured key that commits the draft (default "ctrl+s")
	// link suggestion state
	linkSuggestActive bool
	linkSuggestFrag   string
	linkSuggestSel    int
	linkSuggestList   []string
}

func newEditPane(n *vault.Note, width, height int, notes []*vault.Note, lineNumbers bool, saveKey string) (editPane, tea.Cmd) {
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
	ta, _ = ta.Update(tea.KeyMsg{Type: tea.KeyCtrlHome})
	focusCmd := ta.Focus()

	return editPane{
		note:          n,
		ta:            ta,
		tagsInput:     tags,
		projInput:     proj,
		stateIdx:      si,
		focused:       fldBody,
		wordCount:     countWords(n.Body),
		notes:         notes,
		contentHeight: height,
		saveKey:       saveKey,
	}, focusCmd
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
		if h < 1 {
			h = 1
		}
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
		if after == '`' {
			e.ta, _ = e.ta.Update(tea.KeyMsg{Type: tea.KeyRight})
		} else {
			e.ta.InsertRune('`')
			e.ta.InsertRune('`')
			e.ta, _ = e.ta.Update(tea.KeyMsg{Type: tea.KeyLeft})
		}
		return e, nil

	case "enter":
		e = e.handleEnter()
		e.wordCount = countWords(e.ta.Value())
		return e, nil
	}

	var cmd tea.Cmd
	e.ta, cmd = e.ta.Update(km)
	e.wordCount = countWords(e.ta.Value())
	e = e.refreshLinkSuggest()
	return e, cmd
}

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

	switch {
	case strings.HasPrefix(currentLine, "- [x] "), strings.HasPrefix(currentLine, "- [ ] "):
		e.ta.InsertString("\n- [ ] ")
	case strings.HasPrefix(currentLine, "- "):
		e.ta.InsertString("\n- ")
	case strings.HasPrefix(currentLine, "* "):
		e.ta.InsertString("\n* ")
	default:
		if m := numberedListRe.FindStringSubmatch(currentLine); m != nil {
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
		return e, cmd
	case fldBody:
		cmd := e.ta.Focus()
		return e, cmd
	}
	return e, nil // fldState has no focus command
}

func (e editPane) render(width, height int) string {
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
		Render(fmt.Sprintf("  Words: %d  Lines: %d  Tab: cycle fields", e.wordCount, totalLines))

	parts := []string{titleRow, stateRow, tagsRow, projRow, sep, e.ta.View()}

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
