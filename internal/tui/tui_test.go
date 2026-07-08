package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pkm/internal/index"
	"pkm/internal/vault"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
)

// --- headless simulation helpers ---

func setupTUI(t *testing.T) Model {
	t.Helper()
	dir := t.TempDir()
	v, err := vault.Open(dir)
	if err != nil {
		t.Fatalf("vault.Open: %v", err)
	}
	idx, err := index.Open(filepath.Join(v.Root, ".pkm"))
	if err != nil {
		t.Fatalf("index.Open: %v", err)
	}
	t.Cleanup(func() { idx.Close() })

	// Seed a couple of notes so the list is non-empty.
	n1, _ := v.Create("Docker Basics")
	n2, _ := v.Create("Linux Setup")
	idx.Upsert(n1)
	idx.Upsert(n2)

	m := New(v, idx)
	// Simulate the window-size message bubbletea always sends first.
	model, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return model.(Model)
}

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEscape}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "shift+tab":
		return tea.KeyMsg{Type: tea.KeyShiftTab}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

func click(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}
}

func step(t *testing.T, m Model, msg tea.Msg, label string) Model {
	t.Helper()
	model, _ := m.Update(msg)
	next := model.(Model)
	out := next.View()
	if out == "" {
		t.Errorf("View() empty after %s", label)
	}
	return next
}

func typeString(t *testing.T, m Model, s string) Model {
	t.Helper()
	for _, ch := range s {
		m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}}, "type "+string(ch))
	}
	return m
}

// --- substituteLinks ---

func TestSubstituteLinks(t *testing.T) {
	titles := map[string]bool{"docker": true, "a": true}

	cases := []struct {
		in      string
		wantRef string // expected note title in the first returned linkRef
		working bool   // whether the first link should be a working link
	}{
		// Non-link text is passed through unchanged.
		{"no links", "", false},
		// Working link (title in set): wrapped with working sentinels.
		{"[[Docker]]", "Docker", true},
		// Alias: working link uses alias in display, target in ref.
		{"[[Docker|Container Runtime]]", "Docker", true},
		// Broken link (title not in set): wrapped with broken sentinels.
		{"[[Missing]]", "Missing", false},
		// Unclosed [[ is preserved as-is.
		{"[[unclosed", "", false},
	}

	for _, tc := range cases {
		got, refs := substituteLinks(tc.in, titles)
		// Check that non-link text survives.
		if tc.wantRef == "" {
			if len(refs) != 0 {
				t.Errorf("substituteLinks(%q): want 0 refs, got %d", tc.in, len(refs))
			}
			continue
		}
		if len(refs) == 0 {
			t.Errorf("substituteLinks(%q): want ref %q, got none", tc.in, tc.wantRef)
			continue
		}
		if refs[0].target != tc.wantRef {
			t.Errorf("substituteLinks(%q): ref target = %q, want %q", tc.in, refs[0].target, tc.wantRef)
		}
		// Working links contain the working open sentinel; broken contain the broken open sentinel.
		hasWorking := strings.Contains(got, "")
		hasBroken := strings.Contains(got, "")
		if tc.working && !hasWorking {
			t.Errorf("substituteLinks(%q): expected working sentinel, got %q", tc.in, got)
		}
		if !tc.working && !hasBroken {
			t.Errorf("substituteLinks(%q): expected broken sentinel, got %q", tc.in, got)
		}
	}
}

// --- splitPane history ---

func note(id, title string) *vault.Note {
	return &vault.Note{ID: id, Title: title, State: vault.StateInbox}
}

func TestSplitPaneOpenNote(t *testing.T) {
	sp := newSplitPane()
	n := note("1", "A")
	sp.openNote(n)

	if sp.histIdx != 0 {
		t.Errorf("histIdx: got %d want 0", sp.histIdx)
	}
	if sp.viewer.note != n {
		t.Error("viewer not updated")
	}
	if sp.activeView != viewNote {
		t.Error("activeView not set to viewNote")
	}
}

func TestSplitPaneBackForward(t *testing.T) {
	sp := newSplitPane()
	a, b, c := note("1", "A"), note("2", "B"), note("3", "C")
	sp.openNote(a)
	sp.openNote(b)
	sp.openNote(c)

	// histIdx should be 2 (pointing at C)
	if sp.histIdx != 2 {
		t.Errorf("histIdx after 3 opens: got %d want 2", sp.histIdx)
	}

	// back → B
	if !sp.back() {
		t.Fatal("back() returned false")
	}
	if sp.viewer.note.ID != "2" {
		t.Errorf("after back: got %q want B", sp.viewer.note.Title)
	}

	// back → A
	sp.back()
	if sp.viewer.note.ID != "1" {
		t.Errorf("after 2x back: got %q want A", sp.viewer.note.Title)
	}

	// back at start → false
	if sp.back() {
		t.Error("back() should return false at history start")
	}

	// forward → B
	if !sp.forward() {
		t.Fatal("forward() returned false")
	}
	if sp.viewer.note.ID != "2" {
		t.Errorf("after forward: got %q want B", sp.viewer.note.Title)
	}
}

func TestSplitPaneHistoryTruncatedOnOpen(t *testing.T) {
	sp := newSplitPane()
	a, b, c, d := note("1", "A"), note("2", "B"), note("3", "C"), note("4", "D")
	sp.openNote(a)
	sp.openNote(b)
	sp.openNote(c)

	// go back to B
	sp.back()

	// open D — should truncate C from forward history
	sp.openNote(d)

	if len(sp.history) != 3 {
		t.Errorf("history len: got %d want 3 (A,B,D)", len(sp.history))
	}
	if sp.history[2].ID != "4" {
		t.Errorf("last history entry: got %q want D", sp.history[2].Title)
	}

	// forward should be a no-op (no forward history)
	if sp.forward() {
		t.Error("forward() should return false after truncation")
	}
}

func TestSplitPaneForwardAtEnd(t *testing.T) {
	sp := newSplitPane()
	sp.openNote(note("1", "A"))
	if sp.forward() {
		t.Error("forward() should return false at end")
	}
}

func TestSplitPaneBackEmpty(t *testing.T) {
	sp := newSplitPane()
	if sp.back() {
		t.Error("back() should return false on empty history")
	}
}

// --- palette DSL input ---

func TestPaletteValue(t *testing.T) {
	p := newPalette(nil, nil)
	p.input = "  new \"Docker\"  "
	if got := p.value(); got != "new \"Docker\"" {
		t.Errorf("value() = %q", got)
	}
}

// --- palette completion ---

func TestPaletteVerbSuggestions(t *testing.T) {
	p := newPalette(nil, nil)

	// Empty input → all commands
	if got := p.verbSuggestions(); len(got) != len(allCommands) {
		t.Errorf("empty input: want %d verb suggestions, got %d", len(allCommands), len(got))
	}

	// "n" → "new", "new project", "new template"
	p.input = "n"
	sug := p.verbSuggestions()
	if len(sug) != 3 || sug[0].name != "new" {
		t.Errorf("input 'n': got %v", sug)
	}

	// "xyz" → no match
	p.input = "xyz"
	if got := p.verbSuggestions(); len(got) != 0 {
		t.Errorf("input 'xyz': expected 0 suggestions, got %d", len(got))
	}

	// "new " → past verb stage, verbSuggestions still returns all with prefix "" — but isTypingVerb is false
	p.input = "new "
	if !(!p.isTypingVerb()) {
		t.Error("'new ' should not be in verb-typing stage")
	}
	def, ok := p.currentCmdDef()
	if !ok || def.name != "new" {
		t.Errorf("currentCmdDef for 'new ': got %v, ok=%v", def, ok)
	}
}

func TestPaletteVerbSuggestionsEditingContext(t *testing.T) {
	p := newPalette(nil, nil).withContext(ctxEditing)
	sug := p.verbSuggestions()
	if len(sug) == 0 || sug[0].name != "insert" {
		t.Fatalf("ctxEditing: want \"insert\" ranked first, got %v", sug)
	}

	// Declaration order (no bias) is untouched by an unrelated context.
	def := newPalette(nil, nil).verbSuggestions()
	if def[0].name != allCommands[0].name {
		t.Errorf("ctxDefault: want declaration order, got %v", def)
	}
}

func TestPaletteTabCompletes(t *testing.T) {
	p := newPalette(nil, nil)
	p.input = "n"
	p, _ = p.update(key("tab"))
	if p.input != "new " {
		t.Errorf("tab completion: got %q, want %q", p.input, "new ")
	}
}

func TestPaletteActiveSlot(t *testing.T) {
	p := newPalette(nil, nil)

	// "open " → slotNote
	p.input = "open "
	if got := p.activeSlot(); got != slotNote {
		t.Errorf("'open ': want slotNote, got %v", got)
	}

	// "move docker " → still slotNote (no arrow yet)
	p.input = "move docker "
	if got := p.activeSlot(); got != slotNote {
		t.Errorf("'move docker ': want slotNote, got %v", got)
	}

	// "move docker → " → slotState
	p.input = "move docker → "
	if got := p.activeSlot(); got != slotState {
		t.Errorf("'move docker → ': want slotState, got %v", got)
	}

	// "theme " → slotTheme
	p.input = "theme "
	if got := p.activeSlot(); got != slotTheme {
		t.Errorf("'theme ': want slotTheme, got %v", got)
	}
}

// --- headless render/update simulation ---

func TestHeadlessView(t *testing.T) {
	m := setupTUI(t)
	out := m.View()
	if out == "" {
		t.Fatal("initial View() is empty")
	}
	if !strings.Contains(out, "PKM") {
		t.Errorf("breadcrumb missing from View(): %q", out[:min(len(out), 100)])
	}
}

func TestHeadlessNavigation(t *testing.T) {
	m := setupTUI(t)

	// Note list navigation
	m = step(t, m, key("j"), "j (cursor down)")
	m = step(t, m, key("k"), "k (cursor up)")
	m = step(t, m, key("j"), "j again")

	// Open a note with Enter
	m = step(t, m, key("enter"), "enter (open note)")
	sp := m.splits[m.activeSplit]
	if sp.activeView != viewNote {
		t.Errorf("after Enter: expected viewNote, got %v", sp.activeView)
	}

	// Scroll in note
	m = step(t, m, key("j"), "j (scroll down in note)")
	m = step(t, m, key("k"), "k (scroll up in note)")

	// Go back to list
	m = step(t, m, key("esc"), "esc (back to list)")

	// Switch to sidebar
	m = step(t, m, key("tab"), "tab (→ sidebar)")
	if m.activePane != paneSidebar {
		t.Errorf("after Tab: expected paneSidebar, got %v", m.activePane)
	}

	// Navigate sidebar
	m = step(t, m, key("j"), "j (sidebar down)")
	m = step(t, m, key("k"), "k (sidebar up)")

	// Switch back to pane
	m = step(t, m, key("tab"), "tab (→ pane)")
	if m.activePane != paneMain {
		t.Errorf("after second Tab: expected paneMain, got %v", m.activePane)
	}

	// Shift+Tab mirrors Tab for the sidebar/main toggle (only two states, so
	// "previous" and "next" land on the same target either direction).
	m = step(t, m, key("shift+tab"), "shift+tab (→ sidebar)")
	if m.activePane != paneSidebar {
		t.Errorf("after Shift+Tab: expected paneSidebar, got %v", m.activePane)
	}
	m = step(t, m, key("shift+tab"), "shift+tab (→ pane)")
	if m.activePane != paneMain {
		t.Errorf("after second Shift+Tab: expected paneMain, got %v", m.activePane)
	}
}

// TestEditFromSidebarFocus guards against a regression where pressing "e" to
// open the editor while a note is showing in the main pane but keyboard focus
// is still on the sidebar left activePane out of sync: the edit-mode input
// guard never engaged, so keystrokes (including Shift-hotkeys) leaked to the
// global switch and the sidebar instead of the editor.
func TestEditFromSidebarFocus(t *testing.T) {
	m := setupTUI(t)
	m = step(t, m, key("enter"), "enter (open note)")
	m = step(t, m, key("tab"), "tab (-> sidebar)")
	if m.activePane != paneSidebar {
		t.Fatal("expected focus on sidebar before pressing e")
	}

	m = step(t, m, key("e"), "e (open editor while sidebar focused)")
	sp := m.splits[m.activeSplit]
	if sp.activeView != viewEdit {
		t.Fatalf("expected viewEdit, got %v", sp.activeView)
	}
	if m.activePane != paneMain {
		t.Fatal("activePane was not switched to paneMain on entering the editor")
	}

	// A capital letter that doubles as a global Shift-hotkey must land in the
	// buffer, not reopen the palette or route to the sidebar.
	m = step(t, m, key("N"), "N (type into editor body)")
	if m.showPalette {
		t.Error("Shift-hotkey leaked into the global switch while editing")
	}
	if !strings.Contains(m.splits[m.activeSplit].editor.ta.Value(), "N") {
		t.Error("typed rune did not reach the editor's textarea")
	}
}

func TestHeadlessPalette(t *testing.T) {
	m := setupTUI(t)

	// Open palette
	m = step(t, m, key(":"), "colon (open palette)")
	if !m.showPalette {
		t.Fatal("palette not shown after :")
	}

	// Dropdown should appear in View
	out := m.View()
	if !strings.Contains(out, "▶") {
		t.Error("dropdown indicator '▶' missing from palette view")
	}

	// Type a partial command
	m = step(t, m, key("n"), "n (type 'n')")
	if m.palette.input != "n" {
		t.Errorf("palette input: got %q want %q", m.palette.input, "n")
	}

	// Tab should complete to "new "
	m = step(t, m, key("tab"), "tab (complete 'new ')")
	if m.palette.input != "new " {
		t.Errorf("after tab: palette input = %q, want %q", m.palette.input, "new ")
	}

	// Esc closes palette
	m = step(t, m, key("esc"), "esc (close palette)")
	if m.showPalette {
		t.Error("palette still shown after Esc")
	}
}

func TestHeadlessMouse(t *testing.T) {
	m := setupTUI(t)

	// Click the "▶" glyph on the sidebar Inbox section header — only the
	// glyph column toggles expand/collapse (see sidebarGlyphHit).
	// With pane border, y offsets: breadcrumb(0), top-border(1), SECTIONS(2), blank(3), items(4+)
	// Inbox is items[0] → y=4
	m = step(t, m, click(2, 4), "click sidebar Inbox glyph")
	if !m.sidebar.expanded[vault.StateInbox] {
		t.Error("Inbox should be expanded after glyph click")
	}

	// Clicking the label (not the glyph) opens the section view but must
	// not re-toggle (collapse) the expand state.
	m = step(t, m, click(8, 4), "click sidebar Inbox label")
	if m.sidebar.activeState != vault.StateInbox {
		t.Errorf("sidebar state after label click: got %v", m.sidebar.activeState)
	}
	if !m.sidebar.expanded[vault.StateInbox] {
		t.Error("Inbox should still be expanded after label click")
	}

	// Click on a note in the main pane.
	// With pane border: breadcrumb(0), top-border(1), noteList-padding(2), notes(3+)
	// First note is at y=3.
	l := m.computeLayout()
	noteX := l.sidebarWidth + 5
	m = step(t, m, click(noteX, 3), "click first note")

	// After expanding Inbox (2 notes), the sidebar flat list is:
	// items[0]=Inbox, items[1]=note1, items[2]=note2, items[3]=Projects
	// y=4: Inbox, y=5: note1, y=6: note2, y=7: Projects
	m = step(t, m, click(5, 7), "click sidebar Projects")
	if m.sidebar.activeState != vault.StateProjects {
		t.Errorf("sidebar state after click Projects: got %v", m.sidebar.activeState)
	}
}

// TestHeadlessCheckboxClickToggle covers clicking a task-list checkbox in
// view mode: the click should flip "[ ]" <-> "[x]" in the saved note body.
func TestHeadlessCheckboxClickToggle(t *testing.T) {
	m := setupTUI(t)
	n, err := m.vault.Create("Tasks")
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	n.Body = "- [ ] Task one"
	if err := m.vault.Save(n); err != nil {
		t.Fatalf("save note: %v", err)
	}
	m.index.Upsert(n)
	m.titleSet[strings.ToLower(n.Title)] = true

	sp := &m.splits[m.activeSplit]
	sp.openNote(n)
	l := m.computeLayout()
	sp.viewer = sp.viewer.preRender(l.paneWidth, m.titleSet)

	var bodyLine int
	found := false
	for rl := range sp.viewer.checkboxLines {
		bodyLine = rl
		found = true
		break
	}
	if !found {
		t.Fatalf("no checkbox detected; checkboxLines=%v rendered=%q", sp.viewer.checkboxLines, sp.viewer.rendered)
	}

	// Inverse of handleMouseClick's bodyLine math: y offsets are
	// breadcrumb(0), top-border(1), content starts at y=2.
	y := 2 + sp.viewer.headerLineCount + 1 + bodyLine - sp.viewer.scrollOff
	m = step(t, m, click(l.sidebarWidth+5, y), "click checkbox")

	updated, err := m.vault.FindByTitle("Tasks")
	if err != nil {
		t.Fatalf("reload note: %v", err)
	}
	if !strings.Contains(updated.Body, "[x]") {
		t.Errorf("expected checkbox toggled to [x], got body=%q", updated.Body)
	}
}

// TestViewerCursorMovement exercises the block cursor's arrow-key movement
// and boundary clamping directly against viewerModel, independent of Model
// plumbing. Markdown reflow means raw line count doesn't predict rendered
// line count (e.g. unbroken lines get joined into one paragraph), so this
// finds the actual rendered content line rather than assuming an index.
func TestViewerCursorMovement(t *testing.T) {
	n := note("1", "T")
	n.Body = "A short paragraph.\n\nAnother paragraph here."
	m := newViewer().withNote(n)
	m = m.preRender(60, nil)

	lines := strings.Split(m.rendered, "\n")
	contentRow := -1
	for i, l := range lines {
		if xansi.StringWidth(l) > 0 {
			contentRow = i
			break
		}
	}
	if contentRow == -1 || contentRow+1 >= len(lines) {
		t.Fatalf("expected at least one non-blank rendered line followed by another: %q", lines)
	}

	// Down past the last line clamps instead of going out of bounds.
	for i := 0; i < len(lines)+5; i++ {
		m, _ = m.update(tea.KeyMsg{Type: tea.KeyDown}, 20)
	}
	if m.cursorRow != len(lines)-1 {
		t.Errorf("expected cursor clamped at last line %d, got %d", len(lines)-1, m.cursorRow)
	}

	// Right past end-of-line wraps to the start of the next line.
	m.cursorRow, m.cursorCol = contentRow, 0
	lineWidth := xansi.StringWidth(lines[contentRow])
	for i := 0; i < lineWidth; i++ {
		m, _ = m.update(tea.KeyMsg{Type: tea.KeyRight}, 20)
	}
	m, _ = m.update(tea.KeyMsg{Type: tea.KeyRight}, 20)
	if m.cursorRow != contentRow+1 || m.cursorCol != 0 {
		t.Errorf("expected wrap to (%d,0), got (%d,%d)", contentRow+1, m.cursorRow, m.cursorCol)
	}

	// Left at column 0 wraps back to the end of the previous line.
	m, _ = m.update(tea.KeyMsg{Type: tea.KeyLeft}, 20)
	if m.cursorRow != contentRow || m.cursorCol != lineWidth {
		t.Errorf("expected wrap back to (%d,%d), got (%d,%d)", contentRow, lineWidth, m.cursorRow, m.cursorCol)
	}
}

// TestHeadlessCursorActivateLink covers pressing Enter while the block
// cursor sits on a wikilink: it should navigate to that note, the same as a
// mouse click on the link.
func TestHeadlessCursorActivateLink(t *testing.T) {
	m := setupTUI(t)
	target, err := m.vault.Create("Target Note")
	if err != nil {
		t.Fatalf("create target: %v", err)
	}
	m.index.Upsert(target)
	m.titleSet[strings.ToLower(target.Title)] = true

	n, err := m.vault.Create("Linker")
	if err != nil {
		t.Fatalf("create linker: %v", err)
	}
	n.Body = "[[Target Note]]"
	if err := m.vault.Save(n); err != nil {
		t.Fatalf("save linker: %v", err)
	}
	m.index.Upsert(n)
	m.titleSet[strings.ToLower(n.Title)] = true

	sp := &m.splits[m.activeSplit]
	sp.openNote(n)
	l := m.computeLayout()
	sp.viewer = sp.viewer.preRender(l.paneWidth, m.titleSet)

	found := false
	for rl := range sp.viewer.linkLines {
		sp.viewer.cursorRow = rl
		found = true
		break
	}
	if !found {
		t.Fatalf("no link detected; linkLines=%v rendered=%q", sp.viewer.linkLines, sp.viewer.rendered)
	}
	sp.viewer.cursorCol = 0

	m = step(t, m, key("enter"), "activate link under cursor")

	got := m.splits[m.activeSplit].viewer.note
	if got == nil || got.Title != "Target Note" {
		t.Errorf("expected cursor-activated Enter to open Target Note, got %v", got)
	}
}

// TestHeadlessCursorActivateCheckbox covers pressing Enter while the block
// cursor sits on a task-list checkbox: it should toggle it, the same as a
// mouse click.
func TestHeadlessCursorActivateCheckbox(t *testing.T) {
	m := setupTUI(t)
	n, err := m.vault.Create("Tasks")
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	n.Body = "- [ ] Task one"
	if err := m.vault.Save(n); err != nil {
		t.Fatalf("save note: %v", err)
	}
	m.index.Upsert(n)
	m.titleSet[strings.ToLower(n.Title)] = true

	sp := &m.splits[m.activeSplit]
	sp.openNote(n)
	l := m.computeLayout()
	sp.viewer = sp.viewer.preRender(l.paneWidth, m.titleSet)

	found := false
	for rl := range sp.viewer.checkboxLines {
		sp.viewer.cursorRow = rl
		found = true
		break
	}
	if !found {
		t.Fatalf("no checkbox detected; checkboxLines=%v", sp.viewer.checkboxLines)
	}

	m = step(t, m, key("enter"), "activate checkbox under cursor")

	updated, err := m.vault.FindByTitle("Tasks")
	if err != nil {
		t.Fatalf("reload note: %v", err)
	}
	if !strings.Contains(updated.Body, "[x]") {
		t.Errorf("expected checkbox toggled to [x], got body=%q", updated.Body)
	}
}

// TestHeadlessCursorActivateCodeCopy covers pressing Enter while the block
// cursor sits inside a fenced code block: it should trigger a clipboard-copy
// command and report status, without mutating the note.
func TestHeadlessCursorActivateCodeCopy(t *testing.T) {
	m := setupTUI(t)
	n, err := m.vault.Create("Snippet")
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	n.Body = "```go\nfmt.Println(\"hi\")\n```"
	if err := m.vault.Save(n); err != nil {
		t.Fatalf("save note: %v", err)
	}
	m.index.Upsert(n)
	m.titleSet[strings.ToLower(n.Title)] = true

	sp := &m.splits[m.activeSplit]
	sp.openNote(n)
	l := m.computeLayout()
	sp.viewer = sp.viewer.preRender(l.paneWidth, m.titleSet)

	if len(sp.viewer.codeSpans) == 0 {
		t.Fatalf("no code span detected")
	}
	sp.viewer.cursorRow = sp.viewer.codeSpans[0].startLine

	m = step(t, m, key("enter"), "activate code block under cursor")

	if m.statusMsg != "copied code block" {
		t.Errorf("expected status %q, got %q", "copied code block", m.statusMsg)
	}
}

// typeInPalette feeds a string into the open palette one rune at a time,
// matching how paletteModel.update only accepts single-rune key messages.
func typeInPalette(t *testing.T, m Model, s string) Model {
	t.Helper()
	for _, ch := range s {
		m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}}, "type char")
	}
	return m
}

// TestHeadlessSearchBareEnterOpensResults covers the "no explicit pick" path:
// typing :search <query> and pressing Enter without arrowing through the
// dropdown should open the full results list, not any single note.
func TestHeadlessSearchBareEnterOpensResults(t *testing.T) {
	m := setupTUI(t)
	// setupTUI seeds "Docker Basics" and "Linux Setup".
	m = step(t, m, key(":"), "open palette")
	m = typeInPalette(t, m, "search docker")
	m = step(t, m, key("enter"), "submit search (no nav)")

	if m.showPalette {
		t.Fatal("palette still open after Enter")
	}
	sp := m.splits[m.activeSplit]
	if sp.activeView != viewList {
		t.Fatalf("expected results list view, got %v", sp.activeView)
	}
	if len(m.noteList.notes) != 1 || m.noteList.notes[0].Title != "Docker Basics" {
		t.Errorf("expected [Docker Basics], got %v", m.noteList.notes)
	}
}

// TestHeadlessSearchNavigatedEnterOpensNote covers the "explicit pick" path:
// arrowing to a dropdown hit before pressing Enter opens that note directly.
func TestHeadlessSearchNavigatedEnterOpensNote(t *testing.T) {
	m := setupTUI(t)
	m = step(t, m, key(":"), "open palette")
	m = typeInPalette(t, m, "search docker")
	m = step(t, m, key("down"), "navigate to the first hit")
	m = step(t, m, key("enter"), "submit search (navigated)")

	if m.showPalette {
		t.Fatal("palette still open after Enter")
	}
	sp := m.splits[m.activeSplit]
	if sp.activeView != viewNote || sp.viewer.note == nil {
		t.Fatalf("expected the hit opened directly, got view=%v", sp.activeView)
	}
	if sp.viewer.note.Title != "Docker Basics" {
		t.Errorf("expected Docker Basics opened, got %q", sp.viewer.note.Title)
	}
}

// TestHeadlessSearchBackReturnsToResults covers back-navigation out of a
// result: it should land on the results list again, not a fresh search.
// Crucially, the split already has an unrelated note open before the search
// runs (as it would after session restore) — a regression test for a real
// bug where "back" popped into that pre-existing history instead of
// stopping at the search results, because openNote merges into whatever
// history already existed.
func TestHeadlessSearchBackReturnsToResults(t *testing.T) {
	m := setupTUI(t)
	linux, err := m.vault.FindByTitle("Linux Setup")
	if err != nil {
		t.Fatalf("find seed note: %v", err)
	}
	m.splits[m.activeSplit].openNote(linux)

	m = step(t, m, key(":"), "open palette")
	m = typeInPalette(t, m, "search docker")
	m = step(t, m, key("down"), "navigate to the first hit")
	m = step(t, m, key("enter"), "open the hit")

	m = step(t, m, key("esc"), "back out of the note")

	sp := m.splits[m.activeSplit]
	if sp.activeView != viewList {
		t.Fatalf("expected back to land on the results list, got %v (note=%v)", sp.activeView, sp.viewer.note)
	}
	if len(m.noteList.notes) != 1 || m.noteList.notes[0].Title != "Docker Basics" {
		t.Errorf("expected results still [Docker Basics], got %v", m.noteList.notes)
	}
}

func TestHeadlessCreateNote(t *testing.T) {
	m := setupTUI(t)

	// :new "Test Note" via palette
	m = step(t, m, key(":"), "open palette")
	for _, ch := range `new "Test Note"` {
		m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}}, "type char")
	}
	if m.palette.input != `new "Test Note"` {
		t.Errorf("palette input: got %q", m.palette.input)
	}
	m = step(t, m, key("enter"), "submit command")
	if m.showPalette {
		t.Error("palette still open after Enter")
	}
	sp := m.splits[m.activeSplit]
	if sp.activeView != viewNote || sp.viewer.note == nil {
		t.Error("expected note to be open after :new")
	}
	if sp.viewer.note.Title != "Test Note" {
		t.Errorf("note title: got %q want %q", sp.viewer.note.Title, "Test Note")
	}
}

// TestEditorCtrlSpaceInsertStaysInEditor covers Phase 2 of the editor/palette
// integration: Ctrl+Space opens the palette on top of a still-live editor,
// and :insert writes into the open buffer without saving or leaving edit mode.
// TestEditorTripleBacktickOpensFence guards against the reported bug where
// typing three backticks produced a four-backtick autoclose. Typing "```"
// should expand into a full fenced block ("```\n\n```") with the cursor
// parked on the blank line inside it, not leave a stray closing backtick.
func TestEditorTripleBacktickOpensFence(t *testing.T) {
	m := setupTUI(t)
	m = step(t, m, key("enter"), "open first note")
	m = step(t, m, key("e"), "e (edit)")
	if m.splits[m.activeSplit].activeView != viewEdit {
		t.Fatal("expected viewEdit")
	}

	m = typeString(t, m, "```")

	body := m.splits[m.activeSplit].editor.ta.Value()
	if strings.Count(body, "`") != 6 {
		t.Errorf("expected exactly two fences (6 backticks), got %d in body = %q", strings.Count(body, "`"), body)
	}
	if !strings.Contains(body, "```\n\n```") {
		t.Errorf("expected an opened fence block, body = %q", body)
	}
}

// TestEditorFooterShowsDirtyAndSavedTime covers moving the "saved" status out
// of the global hotkey toolbar into a persistent footer inside the document
// pane: the editor footer shows an "Unsaved changes" marker while the draft
// diverges from the saved note, and a "Last saved" timestamp that only
// updates once the draft is actually committed.
func TestEditorFooterShowsDirtyAndSavedTime(t *testing.T) {
	m := setupTUI(t)
	m = step(t, m, key("enter"), "open first note")
	m = step(t, m, key("e"), "e (edit)")

	sp := m.splits[m.activeSplit]
	savedAt := sp.editor.note.Updated
	if sp.editor.dirty() {
		t.Fatal("fresh editor should not be dirty")
	}
	body := sp.editor.render(100, 20)
	if strings.Contains(body, "Unsaved changes") {
		t.Error("unexpected dirty marker before any edit")
	}
	if !strings.Contains(body, "Last saved: "+savedAt.Format("15:04:05")) {
		t.Errorf("expected last-saved timestamp in footer, got: %q", body)
	}

	m = typeString(t, m, "x")
	sp = m.splits[m.activeSplit]
	if !sp.editor.dirty() {
		t.Fatal("editor should be dirty after typing")
	}
	if !strings.Contains(sp.editor.render(100, 20), "Unsaved changes") {
		t.Error("expected dirty marker after typing")
	}
	// Regression guard: a narrow pane must degrade the footer to a single
	// line rather than wrap it (a wrapped footer pushes the pane's content
	// past its allotted height — see editPane.footerText / viewer's
	// footerRow). Every degradation stage must stay within width.
	for _, w := range []int{100, 60, 40, 25, 15, 8} {
		line := sp.editor.footerText(w, 5)
		if lipgloss.Width(line) > w {
			t.Errorf("footerText(%d) produced a line %d cells wide: %q", w, lipgloss.Width(line), line)
		}
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeyCtrlS}, "ctrl+s (save)")
	if m.statusMsg != "" {
		t.Errorf("save should not set the global toast status, got %q", m.statusMsg)
	}
	sp = m.splits[m.activeSplit]
	if sp.activeView != viewNote {
		t.Fatal("expected to land back in the viewer after save")
	}
	if sp.viewer.note.Updated.Before(savedAt) {
		t.Error("expected Updated timestamp not to move backward after save")
	}
	viewerBody := sp.viewer.render(60, 20, true)
	if !strings.Contains(viewerBody, "Last saved: "+sp.viewer.note.Updated.Format("15:04:05")) {
		t.Errorf("expected last-saved timestamp in viewer footer, got: %q", viewerBody)
	}
}

func TestEditorCtrlSpaceInsertStaysInEditor(t *testing.T) {
	m := setupTUI(t)
	tmpl, err := m.vault.Create("Meeting Notes")
	if err != nil {
		t.Fatalf("create template: %v", err)
	}
	tmpl.Tags = []string{"template"}
	tmpl.Body = "## Agenda"
	if err := m.vault.Save(tmpl); err != nil {
		t.Fatalf("save template: %v", err)
	}
	m.index.Upsert(tmpl)

	m = step(t, m, key("enter"), "open first note")
	m = step(t, m, key("e"), "e (edit)")
	if m.splits[m.activeSplit].activeView != viewEdit {
		t.Fatal("expected viewEdit")
	}
	m = typeString(t, m, "draft text")

	undoBefore := len(m.undoStack)
	m = step(t, m, tea.KeyMsg{Type: tea.KeyCtrlAt}, "ctrl+space (open palette mid-edit)")
	if !m.showPalette {
		t.Fatal("expected palette to open")
	}
	if m.splits[m.activeSplit].activeView != viewEdit {
		t.Fatal("editor should stay open behind the palette, draft untouched")
	}

	m = typeString(t, m, "insert Meeting Notes")
	m = step(t, m, key("enter"), "submit :insert")

	if m.showPalette {
		t.Error("palette should close after running the command")
	}
	sp := m.splits[m.activeSplit]
	if sp.activeView != viewEdit {
		t.Fatalf("expected to land back in the editor, got %v", sp.activeView)
	}
	body := sp.editor.ta.Value()
	if !strings.Contains(body, "draft text") {
		t.Errorf("live draft lost, body = %q", body)
	}
	if !strings.Contains(body, "## Agenda") {
		t.Errorf("template not inserted, body = %q", body)
	}
	if len(m.undoStack) != undoBefore {
		t.Error(":insert mid-edit should not have saved (no new undo record)")
	}
}

// TestEditorCtrlSpaceNonInsertCommitsAndExits covers the safety half of Phase
// 2: any command other than :insert commits the draft (so it can't be
// clobbered) and exits to the viewer, matching pre-Phase-2 behavior.
func TestEditorCtrlSpaceNonInsertCommitsAndExits(t *testing.T) {
	m := setupTUI(t)
	m = step(t, m, key("enter"), "open first note")
	title := m.splits[m.activeSplit].viewer.note.Title
	m = step(t, m, key("e"), "e (edit)")
	m = typeString(t, m, "draft text")

	m = step(t, m, tea.KeyMsg{Type: tea.KeyCtrlAt}, "ctrl+space (open palette mid-edit)")
	m = typeString(t, m, "archive "+title)
	undoBefore := len(m.undoStack)
	m = step(t, m, key("enter"), "submit :archive")

	if m.showPalette {
		t.Error("palette should close after running the command")
	}
	sp := m.splits[m.activeSplit]
	if sp.activeView != viewNote {
		t.Fatalf("expected to exit to viewNote after a non-insert command, got %v", sp.activeView)
	}
	if len(m.undoStack) != undoBefore+1 {
		t.Error("expected the draft to be committed (one new undo record) before the command ran")
	}
	if !strings.Contains(sp.viewer.note.Body, "draft text") {
		t.Errorf("committed draft lost, body = %q", sp.viewer.note.Body)
	}
	n, err := m.vault.FindByTitle(title)
	if err != nil {
		t.Fatalf("FindByTitle: %v", err)
	}
	if n.State != vault.StateArchive {
		t.Errorf("note state = %v, want %v", n.State, vault.StateArchive)
	}
}

// TestEditorCtrlSpaceEscCancelsPaletteKeepsDraft checks that dismissing the
// palette without running a command (Esc) neither saves nor discards the
// in-progress edit — it just closes the overlay.
func TestEditorCtrlSpaceEscCancelsPaletteKeepsDraft(t *testing.T) {
	m := setupTUI(t)
	m = step(t, m, key("enter"), "open first note")
	m = step(t, m, key("e"), "e (edit)")
	m = typeString(t, m, "draft text")

	undoBefore := len(m.undoStack)
	m = step(t, m, tea.KeyMsg{Type: tea.KeyCtrlAt}, "ctrl+space (open palette mid-edit)")
	m = step(t, m, key("esc"), "esc (cancel palette)")

	if m.showPalette {
		t.Error("palette should be closed after Esc")
	}
	sp := m.splits[m.activeSplit]
	if sp.activeView != viewEdit {
		t.Fatalf("expected to remain in the editor, got %v", sp.activeView)
	}
	if !strings.Contains(sp.editor.ta.Value(), "draft text") {
		t.Errorf("draft lost after cancelling the palette, body = %q", sp.editor.ta.Value())
	}
	if len(m.undoStack) != undoBefore {
		t.Error("cancelling the palette should not have saved")
	}
}

func TestHeadlessSplitPane(t *testing.T) {
	m := setupTUI(t)

	// Open a note first
	m = step(t, m, key("enter"), "open note")

	// :split
	m = step(t, m, key(":"), "open palette")
	for _, ch := range "split" {
		m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}}, "type")
	}
	m = step(t, m, key("enter"), "submit :split")

	if len(m.splits) != 2 {
		t.Errorf("expected 2 splits, got %d", len(m.splits))
	}

	// Ctrl+W cycles pane focus
	m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{23}, Alt: false}, "ctrl+w")

	// :close
	m = step(t, m, key(":"), "open palette")
	for _, ch := range "close" {
		m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}}, "type")
	}
	m = step(t, m, key("enter"), "submit :close")

	if len(m.splits) != 1 {
		t.Errorf("expected 1 split after close, got %d", len(m.splits))
	}
}

func TestSidebarExpandCollapse(t *testing.T) {
	m := setupTUI(t)

	// Switch to sidebar focus.
	m = step(t, m, key("tab"), "tab → sidebar")
	if m.activePane != paneSidebar {
		t.Fatal("expected paneSidebar")
	}

	// Cursor starts at Inbox (item 0). Enter should expand it.
	m = step(t, m, key("enter"), "enter (expand Inbox)")
	if !m.sidebar.expanded[vault.StateInbox] {
		t.Fatal("Inbox should be expanded after Enter")
	}
	// Two notes were seeded, so items should be: Inbox, note1, note2, Projects, ..., #templates
	items := m.sidebar.items()
	if len(items) != len(vault.AllStates)+1+2 {
		t.Errorf("expected %d items after expand, got %d", len(vault.AllStates)+1+2, len(items))
	}
	if items[1].isSection {
		t.Error("items[1] should be a note, not a section")
	}

	// Move cursor down to a note and select it — should open in main pane.
	m = step(t, m, key("j"), "j → note item")
	m = step(t, m, key("enter"), "enter (open note from sidebar)")
	if m.activePane != paneMain {
		t.Error("should switch to main pane after opening note from sidebar")
	}
	if m.splits[m.activeSplit].activeView != viewNote {
		t.Error("expected viewNote after opening note from sidebar")
	}

	// Switch back to sidebar and collapse Inbox.
	m = step(t, m, key("tab"), "tab → sidebar")
	// cursor should still be on Inbox section (was moved back on Enter)
	// or just navigate back to it.
	if m.sidebar.cursor != 0 {
		// After opening a note, cursor may still be on the note item; navigate back.
		for m.sidebar.cursor > 0 {
			m = step(t, m, key("k"), "k")
		}
	}
	m = step(t, m, key("enter"), "enter (collapse Inbox)")
	if m.sidebar.expanded[vault.StateInbox] {
		t.Error("Inbox should be collapsed after second Enter")
	}
	items = m.sidebar.items()
	if len(items) != len(vault.AllStates)+1 {
		t.Errorf("expected %d items after collapse, got %d", len(vault.AllStates)+1, len(items))
	}
}

func TestHeadlessMouseMotionIgnored(t *testing.T) {
	m := setupTUI(t)

	// Mouse motion events should NOT change model state
	before := m.activeSplit
	motionMsg := tea.MouseMsg{X: 10, Y: 10, Action: tea.MouseActionMotion}
	m = step(t, m, motionMsg, "mouse motion")
	if m.activeSplit != before {
		t.Error("mouse motion should not change active split")
	}
}

// TestProjectDetailBridgeEntryDoesNotLeak guards against a regression where
// the project-detail bridge-entry text field had no early-return guard (unlike
// Editor/Palette/Config/PanePicker), so global Shift-hotkeys and "?" and ":"
// hijacked focus instead of being typed into the entry.
func TestProjectDetailBridgeEntryDoesNotLeak(t *testing.T) {
	m := setupTUI(t)
	p, err := m.vault.Projects.Create("Homelab")
	if err != nil {
		t.Fatalf("Projects.Create: %v", err)
	}
	m.splits[m.activeSplit].projectDetail = newProjectDetailPane(p, nil)
	m.splits[m.activeSplit].activeView = viewProjectDetail

	m = step(t, m, key("e"), "e (start bridge entry)")
	if !m.splits[m.activeSplit].projectDetail.editingBridge {
		t.Fatal("expected editingBridge=true after e")
	}

	// Characters that double as global hotkeys must land in the entry field.
	m = step(t, m, key("N"), "N (type into bridge entry)")
	m = step(t, m, key("?"), "? (type into bridge entry)")
	if m.showPalette {
		t.Error("bridge entry leaked a keystroke to the global palette shortcut")
	}
	if m.splits[m.activeSplit].activeView != viewProjectDetail {
		t.Error("bridge entry leaked a keystroke that switched away from Project Detail")
	}
	if got := m.splits[m.activeSplit].projectDetail.bridgeInput; got != "N?" {
		t.Errorf("bridgeInput = %q, want %q", got, "N?")
	}
}

// TestHeadlessDeleteThenUndo covers the full :delete safety net: the file
// must actually disappear from disk, and Ctrl+Z must recreate it with the
// original body and re-register the title so links to it render as working.
func TestHeadlessDeleteThenUndo(t *testing.T) {
	m := setupTUI(t)
	n, err := m.vault.Create("Doomed")
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	n.Body = "some content"
	if err := m.vault.Save(n); err != nil {
		t.Fatalf("save note: %v", err)
	}
	m.index.Upsert(n)
	m.titleSet[strings.ToLower(n.Title)] = true
	path := n.Path

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file to exist before delete: %v", err)
	}

	msg, _ := m.cmdDelete([]string{"Doomed"})
	if !strings.Contains(msg, "deleted") {
		t.Fatalf("cmdDelete message = %q", msg)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected file removed after delete, stat err = %v", err)
	}
	if _, err := m.vault.FindByTitle("Doomed"); err == nil {
		t.Error("expected FindByTitle to fail after delete")
	}
	if m.titleSet[strings.ToLower("Doomed")] {
		t.Error("expected titleSet entry removed after delete")
	}

	m.handleUndo()

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file restored after undo: %v", err)
	}
	restored, err := m.vault.FindByTitle("Doomed")
	if err != nil {
		t.Fatalf("FindByTitle after undo: %v", err)
	}
	if strings.TrimSpace(restored.Body) != "some content" {
		t.Errorf("restored body = %q, want it to contain %q", restored.Body, "some content")
	}
	if !m.titleSet[strings.ToLower("Doomed")] {
		t.Error("expected titleSet entry restored after undo")
	}
}

// TestHeadlessMouseWheelScrollsViewer exercises the wheel-over-viewer path in
// handleMouseWheel end-to-end via Update, guarding the pane-index math shared
// with handleMouseClick (never exercised live since tmux can't send wheel
// events easily).
// TestFooterRowNeverExceedsWidth guards the viewer's "Last saved" + scroll-%
// footer against the same wrap-desyncs-the-border bug fixed in the editor
// footer: at any width, the rendered row must fit within it.
func TestFooterRowNeverExceedsWidth(t *testing.T) {
	left := "Last saved: 15:04:05"
	right := "100%"
	for _, w := range []int{60, 30, 20, 10, 4, 1, 0} {
		row := footerRow(w, left, right)
		if lipgloss.Width(row) > w {
			t.Errorf("footerRow(%d): got width %d, row = %q", w, lipgloss.Width(row), row)
		}
	}
}

func TestHeadlessMouseWheelScrollsViewer(t *testing.T) {
	m := setupTUI(t)
	n, err := m.vault.Create("Wheel Target")
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	n.Body = strings.Repeat("line of body text\n", 40)
	if err := m.vault.Save(n); err != nil {
		t.Fatalf("save note: %v", err)
	}
	m.index.Upsert(n)
	m.titleSet[strings.ToLower(n.Title)] = true

	sp := &m.splits[m.activeSplit]
	sp.openNote(n)
	l := m.computeLayout()
	sp.viewer = sp.viewer.preRender(l.paneWidth, m.titleSet)

	wheelX := l.sidebarWidth + 5
	before := sp.viewer.scrollOff
	m = step(t, m, tea.MouseMsg{X: wheelX, Y: 10, Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown}, "wheel down over viewer")
	sp = &m.splits[m.activeSplit]
	if sp.viewer.scrollOff != before+1 {
		t.Errorf("scrollOff after wheel down: got %d, want %d", sp.viewer.scrollOff, before+1)
	}

	m = step(t, m, tea.MouseMsg{X: wheelX, Y: 10, Action: tea.MouseActionPress, Button: tea.MouseButtonWheelUp}, "wheel up over viewer")
	sp = &m.splits[m.activeSplit]
	if sp.viewer.scrollOff != before {
		t.Errorf("scrollOff after wheel up: got %d, want %d", sp.viewer.scrollOff, before)
	}

	// Wheel over the sidebar must move the sidebar cursor instead.
	beforeCursor := m.sidebar.cursor
	m = step(t, m, tea.MouseMsg{X: 2, Y: 5, Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown}, "wheel down over sidebar")
	if m.sidebar.cursor != beforeCursor+1 {
		t.Errorf("sidebar cursor after wheel down: got %d, want %d", m.sidebar.cursor, beforeCursor+1)
	}
	sp = &m.splits[m.activeSplit]
	if sp.viewer.scrollOff != before {
		t.Error("wheel over sidebar should not have scrolled the viewer")
	}
}

// TestHeadlessImportPopover drives the full :import popover flow: opening it
// via the "I" hotkey, typing a path, tabbing to the move/copy toggle and
// flipping it to Copy, tabbing to the destination and cycling it, then
// confirming — and checks both the vault side effect (note created, source
// file preserved since Copy was selected) and that the popover closes.
func TestHeadlessImportPopover(t *testing.T) {
	m := setupTUI(t)

	srcDir := t.TempDir()
	srcPath := filepath.Join(srcDir, "Imported Note.md")
	if err := os.WriteFile(srcPath, []byte("imported body text"), 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}

	m = step(t, m, key("I"), "I (open import popover)")
	if !m.showImport {
		t.Fatal("expected showImport=true after I")
	}

	m = typeString(t, m, srcPath)
	if m.importView.pathInput.Value() != srcPath {
		t.Fatalf("path input = %q, want %q", m.importView.pathInput.Value(), srcPath)
	}

	m = step(t, m, key("tab"), "tab (-> move/copy)")
	if m.importView.focused != impFldMove {
		t.Fatalf("expected focus on impFldMove, got %v", m.importView.focused)
	}
	if !m.importView.move {
		t.Fatal("expected move=true (default) before toggling")
	}
	m = step(t, m, key(" "), "space (toggle to copy)")
	if m.importView.move {
		t.Fatal("expected move=false after space toggle")
	}

	m = step(t, m, key("tab"), "tab (-> destination)")
	if m.importView.focused != impFldDest {
		t.Fatalf("expected focus on impFldDest, got %v", m.importView.focused)
	}
	m = step(t, m, key("right"), "right (cycle destination)")

	m = step(t, m, key("tab"), "tab (-> confirm)")
	if m.importView.focused != impFldConfirm {
		t.Fatalf("expected focus on impFldConfirm, got %v", m.importView.focused)
	}
	m = step(t, m, key("enter"), "enter (confirm import)")

	if m.showImport {
		t.Fatalf("expected popover closed after successful import, errMsg=%q", m.importView.errMsg)
	}
	n, err := m.vault.FindByTitle("Imported Note")
	if err != nil {
		t.Fatalf("FindByTitle: %v", err)
	}
	if !strings.Contains(n.Body, "imported body text") {
		t.Errorf("imported body missing, got %q", n.Body)
	}
	if _, err := os.Stat(srcPath); err != nil {
		t.Errorf("expected source file preserved (Copy was selected): %v", err)
	}
	sp := m.splits[m.activeSplit]
	if sp.activeView != viewNote || sp.viewer.note == nil || sp.viewer.note.ID != n.ID {
		t.Error("expected the imported note to be opened in the active split")
	}
}

// TestHeadlessImportPopoverEsc guards that Esc cancels the popover without
// touching the vault.
func TestHeadlessImportPopoverEsc(t *testing.T) {
	m := setupTUI(t)
	m = step(t, m, key("I"), "I (open import popover)")
	m = step(t, m, key("esc"), "esc (cancel)")
	if m.showImport {
		t.Error("expected showImport=false after Esc")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// --- benchmarks ---

func BenchmarkViewListOnly(b *testing.B) {
	dir := b.TempDir()
	v, _ := vault.Open(dir)
	idx, _ := index.Open(filepath.Join(v.Root, ".pkm"))
	defer idx.Close()
	n1, _ := v.Create("Docker Basics")
	n2, _ := v.Create("Linux Setup")
	idx.Upsert(n1)
	idx.Upsert(n2)

	m := New(v, idx)
	model, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = model.(Model)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.View()
	}
}

func BenchmarkViewWithNoteOpen(b *testing.B) {
	dir := b.TempDir()
	v, _ := vault.Open(dir)
	idx, _ := index.Open(filepath.Join(v.Root, ".pkm"))
	defer idx.Close()
	n, _ := v.Create("Test Note")
	n.Body = strings.Repeat("# Heading\n\nSome paragraph text here. [[Link]] to another note.\n\n", 20)
	v.Save(n)
	idx.Upsert(n)

	m := New(v, idx)
	model, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = model.(Model)
	model, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = model.(Model)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.View()
	}
}
