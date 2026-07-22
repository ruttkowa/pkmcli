package tui

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"pkm/internal/index"
	"pkm/internal/vault"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
)

// orderedCheckboxRawLines returns the raw body line index of each checkbox
// in top-to-bottom RENDERED order (checkboxLines is keyed by rendered row,
// so map iteration order says nothing about display order on its own).
func orderedCheckboxRawLines(checkboxLines map[int]int) []int {
	rows := make([]int, 0, len(checkboxLines))
	for r := range checkboxLines {
		rows = append(rows, r)
	}
	sort.Ints(rows)
	raws := make([]int, len(rows))
	for i, r := range rows {
		raws[i] = checkboxLines[r]
	}
	return raws
}

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

// --- task line format (completion date + result, issue #11) ---

func TestParseTaskLine(t *testing.T) {
	cases := []struct {
		in         string
		wantText   string
		wantDate   string
		wantResult string
	}{
		{" Plain task", "Plain task", "", ""},
		{" Task ✅ 2026-07-10", "Task", "2026-07-10", ""},
		{" Task --> shipped in v2", "Task", "", "shipped in v2"},
		{" Task ✅ 2026-07-10 --> shipped in v2", "Task", "2026-07-10", "shipped in v2"},
		// Tolerate the non-canonical order too (result before date).
		{" Task --> shipped in v2 ✅ 2026-07-10", "Task", "2026-07-10", "shipped in v2"},
		// Result may itself be a wikilink.
		{" Read paper --> [[202606241530 Notes]]", "Read paper", "", "[[202606241530 Notes]]"},
	}
	for _, tc := range cases {
		text, date, result := parseTaskLine(tc.in)
		if text != tc.wantText || date != tc.wantDate || result != tc.wantResult {
			t.Errorf("parseTaskLine(%q) = (%q, %q, %q), want (%q, %q, %q)",
				tc.in, text, date, result, tc.wantText, tc.wantDate, tc.wantResult)
		}
	}
}

func TestFormatTaskLineRoundTrip(t *testing.T) {
	cases := []struct {
		text, date, result string
		want               string
	}{
		{"Plain task", "", "", " Plain task"},
		{"Task", "2026-07-10", "", " Task ✅ 2026-07-10"},
		{"Task", "", "shipped in v2", " Task --> shipped in v2"},
		{"Task", "2026-07-10", "shipped in v2", " Task ✅ 2026-07-10 --> shipped in v2"},
	}
	for _, tc := range cases {
		got := formatTaskLine(tc.text, tc.date, tc.result)
		if got != tc.want {
			t.Errorf("formatTaskLine(%q, %q, %q) = %q, want %q", tc.text, tc.date, tc.result, got, tc.want)
		}
		// Round-trip: formatting then parsing must recover the same fields.
		text, date, result := parseTaskLine(got)
		if text != tc.text || date != tc.date || result != tc.result {
			t.Errorf("round-trip parseTaskLine(formatTaskLine(...)) = (%q, %q, %q), want (%q, %q, %q)",
				text, date, result, tc.text, tc.date, tc.result)
		}
	}
}

func TestToggleCheckboxLineStampsAndStripsDate(t *testing.T) {
	body := "- [ ] Task A"
	got, ok := toggleCheckboxLine(body, 0)
	if !ok {
		t.Fatalf("toggle 1: ok = false")
	}
	today := timeNow().Format("2006-01-02")
	want := "- [x] Task A ✅ " + today
	if got != want {
		t.Fatalf("toggle 1: got %q, want %q", got, want)
	}

	// Toggling back off must strip the date, not accumulate a second one.
	got, ok = toggleCheckboxLine(got, 0)
	if !ok {
		t.Fatalf("toggle 2: ok = false")
	}
	if got != "- [ ] Task A" {
		t.Fatalf("toggle 2: got %q, want %q", got, "- [ ] Task A")
	}
}

func TestToggleCheckboxLinePreservesResult(t *testing.T) {
	body := "- [ ] Task A --> [[Some Note]]"
	got, ok := toggleCheckboxLine(body, 0)
	if !ok {
		t.Fatalf("toggle: ok = false")
	}
	today := timeNow().Format("2006-01-02")
	want := "- [x] Task A ✅ " + today + " --> [[Some Note]]"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// TestHeadlessCursorActivateTaskResultLink covers issue #11's requirement
// that a task result which parses as a [[wikilink]] is not just rendered as
// a link (alias-only) but is actually openable via the block cursor + Enter,
// same as any other wikilink in the body.
func TestHeadlessCursorActivateTaskResultLink(t *testing.T) {
	m := setupTUI(t)
	target, err := m.vault.Create("Some Note")
	if err != nil {
		t.Fatalf("create target: %v", err)
	}
	m.index.Upsert(target)
	m.titleSet[strings.ToLower(target.Title)] = true

	n, err := m.vault.Create("Linker")
	if err != nil {
		t.Fatalf("create linker: %v", err)
	}
	n.Body = "- [x] Linked result ✅ 2026-07-10 --> [[Some Note]]"
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
		t.Fatalf("no link detected in task result; linkLines=%v rendered=%q", sp.viewer.linkLines, sp.viewer.rendered)
	}
	sp.viewer.cursorCol = 0

	m = step(t, m, key("enter"), "activate task-result link under cursor")

	got := m.splits[m.activeSplit].viewer.note
	if got == nil || got.Title != "Some Note" {
		t.Errorf("expected cursor-activated Enter to open Some Note via the task result link, got %v", got)
	}
}

func TestToggleCheckboxLineBackwardCompatPlainLine(t *testing.T) {
	// A pre-existing "- [x] text" line with no date/result must still parse
	// and toggle correctly (undone: date-less, since none was ever set).
	body := "- [x] Already done"
	got, ok := toggleCheckboxLine(body, 0)
	if !ok {
		t.Fatalf("ok = false")
	}
	if got != "- [ ] Already done" {
		t.Fatalf("got %q, want %q", got, "- [ ] Already done")
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

// TestHeadlessViewNeverExceedsRequestedHeight guards #18's actual root
// cause: the bottom hotkey/tooltip bar had no width safety net, so at
// terminal widths too narrow for its full chip list, lipgloss word-wrapped
// it onto a second line instead of clipping — silently rendering one row
// taller than the window. A real terminal then has to scroll to show the
// extra row, which desyncs every mouse click's Y coordinate from the row
// the app's layout math assumes (reported as clicks landing "about 1.5
// lines" off the sidebar item they targeted). The rendered frame must be
// exactly m.height lines at every width, in every mode this covers.
func TestHeadlessViewNeverExceedsRequestedHeight(t *testing.T) {
	widths := []int{40, 50, 60, 80, 100, 120, 150, 200}
	for _, w := range widths {
		m := setupTUI(t)
		model, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: 40})
		m = model.(Model)

		checkHeight := func(label string) {
			t.Helper()
			lines := strings.Split(m.View(), "\n")
			if len(lines) != 40 {
				t.Errorf("width=%d [%s]: rendered %d lines, want exactly 40 (delta=%d)", w, label, len(lines), len(lines)-40)
			}
		}

		checkHeight("default")

		m = step(t, m, key(":"), "open palette")
		m = typeInPalette(t, m, "config")
		m = step(t, m, key("enter"), "open config")
		checkHeight("config")
		m = step(t, m, key("esc"), "close config")
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

// TestHeadlessEnterOnEmptyMarkerBreaksOutOfList covers the terminal-safe
// stand-in for Shift+Enter: pressing Enter on a list/task marker with no
// text typed after it clears the marker and drops to a plain blank line,
// instead of auto-continuing the list. Enter on a marker WITH text must
// still continue the list as before.
func TestHeadlessEnterOnEmptyMarkerBreaksOutOfList(t *testing.T) {
	cases := []struct {
		name    string
		typed   string
		want    string
		explain string
	}{
		{
			name:  "checkbox marker with text still continues",
			typed: "- [ ] Task",
			want:  "- [ ] Task\n- [ ] ",
		},
		{
			name:  "dash marker with text still continues",
			typed: "- item",
			want:  "- item\n- ",
		},
		{
			name:  "numbered marker with text still continues",
			typed: "1. item",
			want:  "1. item\n2. ",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := setupTUI(t)
			m = step(t, m, key("enter"), "enter (open note)")
			m = step(t, m, key("e"), "e (open editor)")
			m = typeString(t, m, tc.typed)
			m = step(t, m, key("enter"), "enter (continue list)")
			got := m.splits[m.activeSplit].editor.ta.Value()
			if got != tc.want {
				t.Fatalf("after typing %q + Enter: got %q, want %q", tc.typed, got, tc.want)
			}
		})
	}

	// Now the break-out case: Enter again on the freshly-continued, still-
	// empty marker must clear it rather than adding yet another one.
	breakOutCases := []struct {
		name  string
		typed string
		want  string
	}{
		{name: "empty checkbox marker breaks out", typed: "- [ ] Task", want: "- [ ] Task\n"},
		{name: "empty dash marker breaks out", typed: "- item", want: "- item\n"},
		{name: "empty numbered marker breaks out", typed: "1. item", want: "1. item\n"},
	}
	for _, tc := range breakOutCases {
		t.Run(tc.name, func(t *testing.T) {
			m := setupTUI(t)
			m = step(t, m, key("enter"), "enter (open note)")
			m = step(t, m, key("e"), "e (open editor)")
			m = typeString(t, m, tc.typed)
			m = step(t, m, key("enter"), "enter (continue list, now on empty marker)")
			m = step(t, m, key("enter"), "enter again (break out of empty marker)")
			got := m.splits[m.activeSplit].editor.ta.Value()
			if got != tc.want {
				t.Fatalf("after typing %q + Enter + Enter: got %q, want %q", tc.typed, got, tc.want)
			}
		})
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

// TestViewerRawLineMapNotIdentityAfterWrap covers #22's core requirement:
// the general rendered→raw line map must not be an identity map once a
// paragraph word-wraps — this is exactly where a naive `cursorRow ==
// rawLine` approach silently drifts (see #22's qualification comment).
func TestViewerRawLineMapNotIdentityAfterWrap(t *testing.T) {
	n := note("1", "T")
	longPara := strings.Repeat("supercalifragilistic ", 30)
	// raw lines: 0 heading, 1 blank, 2 long paragraph, 3 blank, 4 wikilink,
	// 5 blank, 6 checkbox, 7 trailing blank.
	n.Body = "# Heading\n\n" + longPara + "\n\n[[Some Link]]\n\n- [ ] a task\n"
	m := newViewer().withNote(n)
	m = m.preRender(40, nil) // narrow width forces the paragraph to wrap

	lines := strings.Split(m.rendered, "\n")
	const paraRawLine = 2
	firstParaRendered := m.renderedLineForRaw(paraRawLine)
	if got := m.rawLineAt(firstParaRendered); got != paraRawLine {
		t.Fatalf("rawLineAt(%d) = %d, want %d (paragraph's own anchor line)", firstParaRendered, got, paraRawLine)
	}

	continuationRendered := firstParaRendered + 1
	if continuationRendered >= len(lines) {
		t.Fatalf("test setup didn't produce a wrapped continuation line after rendered line %d (only %d rendered lines) — increase longPara", firstParaRendered, len(lines))
	}
	if continuationRendered == paraRawLine {
		t.Fatalf("test setup didn't actually diverge from identity: rendered index %d == raw index %d", continuationRendered, paraRawLine)
	}
	if got := m.rawLineAt(continuationRendered); got != paraRawLine {
		t.Errorf("rawLineAt(%d) (wrapped continuation of the paragraph) = %d, want it to still resolve to the paragraph's raw start %d, not identity", continuationRendered, got, paraRawLine)
	}

	if linkRendered, ok := firstKey(m.linkLines); ok {
		if got := m.rawLineAt(linkRendered); got != 4 {
			t.Errorf("rawLineAt(link rendered line %d) = %d, want 4", linkRendered, got)
		}
	}

	checkboxRendered, ok := firstKey(m.checkboxLines)
	if !ok {
		t.Fatalf("expected a checkbox line in the rendered output")
	}
	const wantCheckboxRaw = 6
	if m.checkboxLines[checkboxRendered] != wantCheckboxRaw {
		t.Fatalf("test setup: checkboxLines[%d] = %d, want %d", checkboxRendered, m.checkboxLines[checkboxRendered], wantCheckboxRaw)
	}
	if got := m.rawLineAt(checkboxRendered); got != wantCheckboxRaw {
		t.Errorf("rawLineAt(checkbox rendered line %d) = %d, want %d — general map must agree with checkboxLines, not fall back to an earlier anchor", checkboxRendered, got, wantCheckboxRaw)
	}
}

// firstKey returns an arbitrary key from a map[int]string or map[int]int,
// for tests that just need "the one line this note has" without caring
// which iteration order Go picks.
func firstKey[V any](m map[int]V) (int, bool) {
	for k := range m {
		return k, true
	}
	return 0, false
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

// TestHeadlessCursorStaysPutAcrossReorderingToggle covers the #8/#12
// interaction: toggling a task that causes it to sink to the bottom of its
// block must hold the cursor's SCREEN ROW (not follow the task down), same
// as an ordinary in-place toggle. applyCheckboxToggle's save/restore of
// cursorRow/scrollOff around the re-render is identity-agnostic — it never
// knows which task moved where — so this should already hold; this test
// locks that in for the reordering case specifically.
func TestHeadlessCursorStaysPutAcrossReorderingToggle(t *testing.T) {
	m := setupTUI(t)
	n, err := m.vault.Create("ReorderCursor")
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	n.Body = "- [ ] Task A\n- [ ] Task B\n- [ ] Task C"
	if err := m.vault.Save(n); err != nil {
		t.Fatalf("save note: %v", err)
	}
	m.index.Upsert(n)
	m.titleSet[strings.ToLower(n.Title)] = true

	sp := &m.splits[m.activeSplit]
	sp.openNote(n)
	l := m.computeLayout()
	sp.viewer = sp.viewer.preRender(l.paneWidth, m.titleSet)

	// Cursor on "Task A" (raw line 0) — toggling it done will sink it below
	// B and C, so its screen row should end up showing whatever is now in
	// that slot ("Task B"), not follow "Task A" to the bottom.
	var taskARow = -1
	for rl, raw := range sp.viewer.checkboxLines {
		if raw == 0 {
			taskARow = rl
		}
	}
	if taskARow < 0 {
		t.Fatalf("raw line 0 not found: %v", sp.viewer.checkboxLines)
	}
	sp.viewer.cursorRow = taskARow
	sp.viewer.scrollOff = 0

	m = step(t, m, key("enter"), "toggle Task A (will sink)")

	got := m.splits[m.activeSplit].viewer
	if got.cursorRow != taskARow {
		t.Errorf("cursorRow = %d after reordering toggle, want unchanged %d (screen-stable)", got.cursorRow, taskARow)
	}
	// The row the cursor now sits on should show Task B (which rose into
	// Task A's old slot), not Task A (which sank away).
	if raw, ok := got.checkboxLines[got.cursorRow]; !ok || raw != 1 {
		t.Errorf("row under cursor maps to raw line %v, want raw line 1 (Task B rose into this slot)", raw)
	}
}

// TestHeadlessFinishedTasksSinkWithDuplicateText is issue #12's discriminating
// test: two tasks with IDENTICAL text, only one of which is toggled, in a
// block that also has an already-finished task. This is what turns a
// reorder-index-vs-raw-line-index bug into a silent wrong-line edit — if the
// reorder logic uses the post-reorder walk position instead of preserving
// each task's true raw body line, toggling the second "Buy milk" would
// actually edit the wrong line (indistinguishable from the first by text
// alone). Also covers the un-toggle case: a task un-toggled from the
// finished group must return to the unfinished group.
func TestHeadlessFinishedTasksSinkWithDuplicateText(t *testing.T) {
	m := setupTUI(t)
	n, err := m.vault.Create("Dupes")
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	// raw line 0: unfinished "Buy milk"
	// raw line 1: already finished "Already done"
	// raw line 2: unfinished "Buy milk" (duplicate text of line 0)
	n.Body = "- [ ] Buy milk\n- [x] Already done\n- [ ] Buy milk"
	if err := m.vault.Save(n); err != nil {
		t.Fatalf("save note: %v", err)
	}
	m.index.Upsert(n)
	m.titleSet[strings.ToLower(n.Title)] = true

	sp := &m.splits[m.activeSplit]
	sp.openNote(n)
	l := m.computeLayout()
	sp.viewer = sp.viewer.preRender(l.paneWidth, m.titleSet)

	// Display order must be: both unfinished (original relative order), then
	// the already-finished one sinks to the bottom.
	order := orderedCheckboxRawLines(sp.viewer.checkboxLines)
	want := []int{0, 2, 1}
	if len(order) != len(want) {
		t.Fatalf("checkbox order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("checkbox order = %v, want %v", order, want)
		}
	}

	// Toggle the SECOND "Buy milk" (raw line 2), targeted via its display
	// row per the map — not by text, since the text is ambiguous.
	targetRow := -1
	for rl, raw := range sp.viewer.checkboxLines {
		if raw == 2 {
			targetRow = rl
		}
	}
	if targetRow < 0 {
		t.Fatalf("raw line 2 not found in checkboxLines=%v", sp.viewer.checkboxLines)
	}
	sp.viewer.cursorRow = targetRow

	m = step(t, m, key("enter"), "toggle the second Buy milk")

	updated, err := m.vault.FindByTitle("Dupes")
	if err != nil {
		t.Fatalf("reload note: %v", err)
	}
	// vault's frontmatter round-trip leaves a leading blank line after a
	// save+reload cycle (pre-existing, unrelated to #12) — strip it so the
	// line indices below line up with the raw body we wrote.
	lines := strings.Split(strings.TrimPrefix(updated.Body, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("body = %q, want 3 lines", updated.Body)
	}
	if lines[0] != "- [ ] Buy milk" {
		t.Errorf("raw line 0 (untouched duplicate) = %q, want unchanged %q", lines[0], "- [ ] Buy milk")
	}
	if lines[1] != "- [x] Already done" {
		t.Errorf("raw line 1 (unrelated finished task) = %q, want unchanged %q", lines[1], "- [x] Already done")
	}
	if !strings.HasPrefix(lines[2], "- [x] Buy milk") {
		t.Errorf("raw line 2 (the toggled duplicate) = %q, want it toggled to [x]", lines[2])
	}

	// Re-render: the newly-finished raw line 2 must now sink to the bottom,
	// after the already-finished raw line 1 (finished group preserves
	// original relative order), leaving only raw line 0 unfinished. Strip
	// the reload's leading blank line (see above) so raw indices stay 0/1/2
	// consistent with the first half of this test.
	updated.Body = strings.TrimPrefix(updated.Body, "\n")
	sp.viewer = sp.viewer.withNote(updated)
	sp.viewer = sp.viewer.preRender(l.paneWidth, m.titleSet)
	order = orderedCheckboxRawLines(sp.viewer.checkboxLines)
	want = []int{0, 1, 2}
	if len(order) != len(want) {
		t.Fatalf("post-toggle checkbox order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("post-toggle checkbox order = %v, want %v", order, want)
		}
	}

	// Un-toggle raw line 2: it must pop back OUT of the finished group.
	targetRow = -1
	for rl, raw := range sp.viewer.checkboxLines {
		if raw == 2 {
			targetRow = rl
		}
	}
	if targetRow < 0 {
		t.Fatalf("raw line 2 not found after re-render: %v", sp.viewer.checkboxLines)
	}
	sp.viewer.cursorRow = targetRow
	m = step(t, m, key("enter"), "un-toggle the second Buy milk")

	reverted, err := m.vault.FindByTitle("Dupes")
	if err != nil {
		t.Fatalf("reload note: %v", err)
	}
	revLines := strings.Split(strings.TrimPrefix(reverted.Body, "\n"), "\n")
	if revLines[2] != "- [ ] Buy milk" {
		t.Errorf("raw line 2 after un-toggle = %q, want reverted to %q", revLines[2], "- [ ] Buy milk")
	}
	if revLines[0] != "- [ ] Buy milk" || revLines[1] != "- [x] Already done" {
		t.Errorf("un-toggle mutated an unrelated line: body=%q", reverted.Body)
	}
}

// TestProcessCheckboxesAndCodeDimsWrappedContinuationLines guards #17 at the
// same unit level as TestProcessCheckboxesAndCodeMutesFinishedTasks below,
// for the same reason: lipgloss emits no color codes without a real color
// profile, which a headless test binary doesn't have. A finished task's
// cbMark sentinel only lands on its first rendered line — after glamour
// word-wraps a long task, every continuation line must still be dimmed, and
// the run must stop at the next blank line or task, not bleed into either.
func TestProcessCheckboxesAndCodeDimsWrappedContinuationLines(t *testing.T) {
	styled := func(text string) string {
		return "\x1b[38;5;252m" + text + "\x1b[0m"
	}
	insertCBMark := func(s string) string {
		idx := strings.Index(s, "] ") + len("] ")
		return s[:idx] + cbMark + s[idx:]
	}

	lines := []string{
		insertCBMark(styled("[x] first line of a long wrapped task")),
		styled("continuation line one of the same task"),
		styled("continuation line two of the same task"),
		"", // blank separator — must end the dim run
		insertCBMark(styled("[ ] a following unfinished task")),
	}
	rendered := strings.Join(lines, "\n")

	refs := []checkboxRef{
		{rawLine: 0, checked: true},
		{rawLine: 1, checked: false},
	}

	out, _, _, _, _ := processCheckboxesAndCode(rendered, refs, nil, nil, nil)
	outLines := strings.Split(out, "\n")

	if outLines[0] == lines[0] {
		t.Errorf("finished task's first line was not re-tinted: %q", outLines[0])
	}
	if outLines[1] == lines[1] {
		t.Errorf("wrapped continuation line 1 was not dimmed: %q", outLines[1])
	}
	if xansi.Strip(outLines[1]) != "continuation line one of the same task" {
		t.Errorf("continuation line 1 plain text corrupted: %q", outLines[1])
	}
	if outLines[2] == lines[2] {
		t.Errorf("wrapped continuation line 2 was not dimmed: %q", outLines[2])
	}
	if outLines[3] != lines[3] {
		t.Errorf("blank separator line was altered: %q, want unchanged %q", outLines[3], lines[3])
	}
	wantFollowing := styled("[ ] a following unfinished task")
	if outLines[4] != wantFollowing {
		t.Errorf("following unfinished task line = %q, want unchanged %q (dim run must not bleed past the blank line)", outLines[4], wantFollowing)
	}
}

// TestProcessCheckboxesAndCodeUnfinishedWrappedTaskUnchanged is the inverse
// guard: an unfinished (unchecked) task's wrapped continuation lines must
// never be dimmed, even though they carry no cbMark of their own.
func TestProcessCheckboxesAndCodeUnfinishedWrappedTaskUnchanged(t *testing.T) {
	styled := func(text string) string {
		return "\x1b[38;5;252m" + text + "\x1b[0m"
	}
	insertCBMark := func(s string) string {
		idx := strings.Index(s, "] ") + len("] ")
		return s[:idx] + cbMark + s[idx:]
	}

	firstLine := styled("[ ] first line of a long unfinished task")
	contLine := styled("continuation line of the same unfinished task")
	rendered := strings.Join([]string{insertCBMark(firstLine), contLine}, "\n")
	refs := []checkboxRef{{rawLine: 0, checked: false}}

	out, _, _, _, _ := processCheckboxesAndCode(rendered, refs, nil, nil, nil)
	outLines := strings.Split(out, "\n")

	if outLines[0] != firstLine {
		t.Errorf("unfinished task's first line changed: %q, want unchanged (sentinel-stripped only) %q", outLines[0], firstLine)
	}
	if outLines[1] != contLine {
		t.Errorf("unfinished task's continuation line changed: %q, want unchanged %q", outLines[1], contLine)
	}
}

// TestProcessCheckboxesAndCodeDimStopsAtNextTask guards the "not the next
// block" half of #17: a done task immediately followed (no blank line) by
// another task must dim the first task only, leaving the second untouched
// by the dim run (it gets its own treatment based on its own checked state).
func TestProcessCheckboxesAndCodeDimStopsAtNextTask(t *testing.T) {
	styled := func(text string) string {
		return "\x1b[38;5;252m" + text + "\x1b[0m"
	}
	insertCBMark := func(s string) string {
		idx := strings.Index(s, "] ") + len("] ")
		return s[:idx] + cbMark + s[idx:]
	}

	lines := []string{
		insertCBMark(styled("[x] done task")),
		insertCBMark(styled("[ ] a plain unfinished task right after")),
	}
	rendered := strings.Join(lines, "\n")
	refs := []checkboxRef{
		{rawLine: 0, checked: true},
		{rawLine: 1, checked: false},
	}

	out, _, _, _, _ := processCheckboxesAndCode(rendered, refs, nil, nil, nil)
	outLines := strings.Split(out, "\n")

	if outLines[0] == lines[0] {
		t.Errorf("done task line was not re-tinted: %q", outLines[0])
	}
	want := styled("[ ] a plain unfinished task right after")
	if outLines[1] != want {
		t.Errorf("following unfinished task line = %q, want unchanged %q", outLines[1], want)
	}
}

// TestProcessCheckboxesAndCodeMutesFinishedTasks covers #12's table-like/
// secondary styling requirement directly at the unit level (independent of
// glamour's own color-profile detection, which differs between a real
// render and a raw lipgloss call in a headless test environment): a
// finished task's rendered line must come out re-tinted (different from its
// glamour-rendered input), while an unfinished task's line must pass
// through with only the sentinel stripped, untouched otherwise.
func TestProcessCheckboxesAndCodeMutesFinishedTasks(t *testing.T) {
	unfinishedGlamourLine := "\x1b[38;5;252m[ ] Not done\x1b[0m"
	finishedGlamourLine := "\x1b[38;5;252m[x] Done\x1b[0m"
	insertCBMark := func(s string) string {
		idx := strings.Index(s, "] ") + len("] ")
		return s[:idx] + cbMark + s[idx:]
	}
	rendered := strings.Join([]string{
		insertCBMark(unfinishedGlamourLine),
		insertCBMark(finishedGlamourLine),
	}, "\n")

	refs := []checkboxRef{
		{rawLine: 0, checked: false},
		{rawLine: 1, checked: true},
	}

	out, checkboxLines, _, _, _ := processCheckboxesAndCode(rendered, refs, nil, nil, nil)
	outLines := strings.Split(out, "\n")

	if checkboxLines[0] != 0 || checkboxLines[1] != 1 {
		t.Fatalf("checkboxLines = %v, want {0:0, 1:1}", checkboxLines)
	}

	if outLines[0] != unfinishedGlamourLine {
		t.Errorf("unfinished line = %q, want unchanged %q (only sentinel stripped)", outLines[0], unfinishedGlamourLine)
	}
	if outLines[1] == finishedGlamourLine {
		t.Errorf("finished line was not re-tinted, still equals glamour's original rendering: %q", outLines[1])
	}
	// Re-tinted output must still be plain-decodable back to the same text
	// (highlightPlain strips then re-renders — content must survive).
	if plain := xansi.Strip(outLines[1]); plain != "[x] Done" {
		t.Errorf("finished line's plain text = %q, want %q", plain, "[x] Done")
	}
}

// TestHeadlessSpaceTogglesCheckbox covers pressing Space while the block
// cursor sits anywhere on a task-list line (not just on the checkbox
// itself): it should toggle it the same as Enter. Space on a non-task line
// must be a no-op — it should not fall through to Enter's link/code-copy
// behavior.
func TestHeadlessSpaceTogglesCheckbox(t *testing.T) {
	m := setupTUI(t)
	n, err := m.vault.Create("SpaceTasks")
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	n.Body = "Not a task line\n- [ ] Task one"
	if err := m.vault.Save(n); err != nil {
		t.Fatalf("save note: %v", err)
	}
	m.index.Upsert(n)
	m.titleSet[strings.ToLower(n.Title)] = true

	sp := &m.splits[m.activeSplit]
	sp.openNote(n)
	l := m.computeLayout()
	sp.viewer = sp.viewer.preRender(l.paneWidth, m.titleSet)

	// Space on the plain text line must do nothing.
	plainRow := -1
	for row := range strings.Split(sp.viewer.rendered, "\n") {
		if _, ok := sp.viewer.checkboxLines[row]; !ok {
			plainRow = row
			break
		}
	}
	if plainRow < 0 {
		t.Fatalf("expected a non-checkbox row in rendered body: %q", sp.viewer.rendered)
	}
	sp.viewer.cursorRow = plainRow
	sp.viewer.cursorCol = 3 // mid-line, nowhere near a checkbox
	m = step(t, m, key(" "), "space on plain line")
	unchanged, err := m.vault.FindByTitle("SpaceTasks")
	if err != nil {
		t.Fatalf("reload note: %v", err)
	}
	if strings.Contains(unchanged.Body, "[x]") {
		t.Errorf("space on a non-task line must not toggle anything, got body=%q", unchanged.Body)
	}

	// Space anywhere on the checkbox line, cursor NOT over the box itself,
	// must still toggle it.
	found := false
	for row := range sp.viewer.checkboxLines {
		sp.viewer.cursorRow = row
		found = true
		break
	}
	if !found {
		t.Fatalf("no checkbox detected; checkboxLines=%v", sp.viewer.checkboxLines)
	}
	sp.viewer.cursorCol = 20 // past the "[ ] " marker, into the task text

	m = step(t, m, key(" "), "space over checkbox line")
	updated, err := m.vault.FindByTitle("SpaceTasks")
	if err != nil {
		t.Fatalf("reload note: %v", err)
	}
	if !strings.Contains(updated.Body, "[x]") {
		t.Errorf("expected checkbox toggled to [x] via Space, got body=%q", updated.Body)
	}
}

// TestHeadlessVaultChangedPreservesCursor guards against a regression where
// toggling a checkbox saved the note to disk, the fsnotify watcher fired a
// vaultChangedMsg, and the resulting reload snapped the viewer's cursor and
// scroll position back to the top instead of leaving it where the user was.
func TestHeadlessVaultChangedPreservesCursor(t *testing.T) {
	m := setupTUI(t)
	n, err := m.vault.Create("Long Note")
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

	sp.viewer.cursorRow = 10
	sp.viewer.cursorCol = 2
	sp.viewer.scrollOff = 5

	// Simulate the async reload the watcher triggers after a save elsewhere
	// (e.g. a checkbox toggle) touches the same note.
	reloaded := *n
	model, _ := m.Update(vaultChangedMsg{note: &reloaded})
	m = model.(Model)

	got := m.splits[m.activeSplit].viewer
	if got.cursorRow != 10 || got.cursorCol != 2 || got.scrollOff != 5 {
		t.Errorf("cursor/scroll position not preserved across vaultChangedMsg: got row=%d col=%d scroll=%d, want row=10 col=2 scroll=5", got.cursorRow, got.cursorCol, got.scrollOff)
	}
}

// foldTestBody is shared by the #20 fold tests: a heading with a task and a
// nested sub-heading (to hide when collapsed), followed by a sibling heading
// at the same level containing a link and a task (to prove content AFTER a
// fold still resolves to the correct raw line — the index-shift regression
// the spec calls out).
const foldTestBody = `Intro text before any heading.

## Section A
- [ ] task under A

### Sub A1
content under sub

## Section B
[[Target Note]]
- [ ] task under B`

// foldTestNote creates a note with foldTestBody plus a link target, and
// returns the note opened and pre-rendered in the active split.
func foldTestNote(t *testing.T, m Model) (Model, *vault.Note) {
	t.Helper()
	target, err := m.vault.Create("Target Note")
	if err != nil {
		t.Fatalf("create target: %v", err)
	}
	m.index.Upsert(target)
	m.titleSet[strings.ToLower(target.Title)] = true

	n, err := m.vault.Create("Foldable")
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	n.Body = foldTestBody
	if err := m.vault.Save(n); err != nil {
		t.Fatalf("save note: %v", err)
	}
	m.index.Upsert(n)
	m.titleSet[strings.ToLower(n.Title)] = true

	sp := &m.splits[m.activeSplit]
	sp.openNote(n)
	l := m.computeLayout()
	sp.viewer = sp.viewer.preRender(l.paneWidth, m.titleSet)
	return m, n
}

// TestHiddenLinesForFoldHidesNestedHeadingRegardlessOfOwnState guards #20's
// core folding rule directly: collapsing "## Section A" (raw line 2) must
// hide everything up to but not including "## Section B" (raw line 8) —
// including the nested "### Sub A1" (raw line 5) and its content, regardless
// of Sub A1's own fold state, which is never set here.
func TestHiddenLinesForFoldHidesNestedHeadingRegardlessOfOwnState(t *testing.T) {
	lines := strings.Split(foldTestBody, "\n")
	if lines[2] != "## Section A" || lines[8] != "## Section B" {
		t.Fatalf("test body layout changed, fix line indices: %q / %q", lines[2], lines[8])
	}

	hidden := hiddenLinesForFold(lines, map[int]bool{2: true})

	wantHidden := []int{3, 4, 5, 6, 7}
	for _, i := range wantHidden {
		if !hidden[i] {
			t.Errorf("line %d (%q) should be hidden, wasn't", i, lines[i])
		}
	}
	wantVisible := []int{0, 1, 2, 8, 9, 10}
	for _, i := range wantVisible {
		if hidden[i] {
			t.Errorf("line %d (%q) should be visible, was hidden", i, lines[i])
		}
	}
}

// TestHeadlessFoldLinkAndCheckboxBelowFoldStillResolve is the index-shift
// regression #20's spec calls out by name: with "## Section A" collapsed, a
// link and a checkbox in the following "## Section B" (raw lines 9 and 10)
// must still map to the correct raw line / target — not shifted by however
// many lines were hidden above them.
func TestHeadlessFoldLinkAndCheckboxBelowFoldStillResolve(t *testing.T) {
	m := setupTUI(t)
	m, _ = foldTestNote(t, m)
	sp := &m.splits[m.activeSplit]

	sp.viewer.folded = map[int]bool{2: true}
	sp.viewer.rendered = ""
	l := m.computeLayout()
	sp.viewer = sp.viewer.preRender(l.paneWidth, m.titleSet)

	foundLink := false
	for _, target := range sp.viewer.linkLines {
		if target == "Target Note" {
			foundLink = true
		}
	}
	if !foundLink {
		t.Errorf("expected a link to Target Note after the fold, linkLines=%v rendered=%q", sp.viewer.linkLines, sp.viewer.rendered)
	}

	foundCheckbox := false
	for _, raw := range sp.viewer.checkboxLines {
		if raw == 10 {
			foundCheckbox = true
		}
		if raw == 3 {
			t.Errorf("checkbox for the HIDDEN raw line 3 (task under A) must not appear in checkboxLines")
		}
	}
	if !foundCheckbox {
		t.Errorf("expected a checkbox mapping to raw line 10 (task under B), checkboxLines=%v", sp.viewer.checkboxLines)
	}

	// The hidden task's text must not appear anywhere in the rendered output.
	if strings.Contains(xansi.Strip(sp.viewer.rendered), "task under A") {
		t.Error("hidden task text leaked into the rendered output")
	}
	if !strings.Contains(xansi.Strip(sp.viewer.rendered), "task under B") {
		t.Error("visible task text (after the fold) missing from rendered output")
	}
}

// TestHeadlessFoldCollapseExpandRoundTrips guards that toggling a fold off
// again produces byte-identical output to before it was folded.
func TestHeadlessFoldCollapseExpandRoundTrips(t *testing.T) {
	m := setupTUI(t)
	m, _ = foldTestNote(t, m)
	sp := &m.splits[m.activeSplit]
	original := sp.viewer.rendered

	m.applyFold(sp, 2, true)
	if sp.viewer.rendered == original {
		t.Fatal("expected the rendered output to change after collapsing")
	}

	m.applyFold(sp, 2, false)
	if sp.viewer.rendered != original {
		t.Errorf("collapse/expand round-trip mismatch:\noriginal=%q\ngot=     %q", original, sp.viewer.rendered)
	}
}

// TestHeadlessFoldKeyboardLeftRightOnHeading drives the actual interactive
// path end-to-end: put the cursor on a heading's rendered line, press Left
// (collapse) then Right (expand) through Model.Update, and confirm the fold
// state and rendered content follow.
func TestHeadlessFoldKeyboardLeftRightOnHeading(t *testing.T) {
	m := setupTUI(t)
	m, _ = foldTestNote(t, m)
	sp := &m.splits[m.activeSplit]

	headingRow := -1
	for rl, raw := range sp.viewer.headingLines {
		if raw == 2 {
			headingRow = rl
		}
	}
	if headingRow < 0 {
		t.Fatalf("Section A heading not found in headingLines=%v", sp.viewer.headingLines)
	}
	m.splits[m.activeSplit].viewer.cursorRow = headingRow

	m = step(t, m, tea.KeyMsg{Type: tea.KeyLeft}, "left (collapse heading)")
	if !m.splits[m.activeSplit].viewer.folded[2] {
		t.Fatal("expected raw line 2 folded after Left on the heading")
	}
	if strings.Contains(xansi.Strip(m.splits[m.activeSplit].viewer.rendered), "task under A") {
		t.Error("hidden task text still present after collapsing via Left")
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeyRight}, "right (expand heading)")
	if m.splits[m.activeSplit].viewer.folded[2] {
		t.Fatal("expected raw line 2 unfolded after Right on the heading")
	}
	if !strings.Contains(xansi.Strip(m.splits[m.activeSplit].viewer.rendered), "task under A") {
		t.Error("task text missing after expanding via Right")
	}
}

// TestHeadlessLeftRightOnNonHeadingStillMovesCursor guards that the heading
// guard in Left/Right doesn't leak into ordinary cursor movement.
func TestHeadlessLeftRightOnNonHeadingStillMovesCursor(t *testing.T) {
	m := setupTUI(t)
	m, _ = foldTestNote(t, m)
	sp := &m.splits[m.activeSplit]

	// Raw line 0 ("Intro text...") is not a heading; find its rendered row.
	nonHeadingRow := -1
	for i, l := range strings.Split(sp.viewer.rendered, "\n") {
		if strings.Contains(l, "Intro text") {
			nonHeadingRow = i
			break
		}
	}
	if nonHeadingRow < 0 {
		t.Fatalf("intro line not found in rendered=%q", sp.viewer.rendered)
	}
	m.splits[m.activeSplit].viewer.cursorRow = nonHeadingRow
	m.splits[m.activeSplit].viewer.cursorCol = 0

	m = step(t, m, tea.KeyMsg{Type: tea.KeyRight}, "right (move cursor, not a heading)")
	got := m.splits[m.activeSplit].viewer
	if got.cursorCol != 1 || got.cursorRow != nonHeadingRow {
		t.Errorf("expected ordinary cursor movement (col 0->1, same row), got row=%d col=%d", got.cursorRow, got.cursorCol)
	}
	if len(got.folded) != 0 {
		t.Errorf("expected no fold state change from Right on a non-heading line, folded=%v", got.folded)
	}
}

// TestHeadlessFoldCollapseNearEndNoPanic guards the clamp trap: with the
// cursor sitting on the last rendered line, collapsing a heading near the
// top must clamp cursor/scroll into the shortened content rather than
// leaving them out of range (which would panic on the next render).
func TestHeadlessFoldCollapseNearEndNoPanic(t *testing.T) {
	m := setupTUI(t)
	m, _ = foldTestNote(t, m)
	sp := &m.splits[m.activeSplit]

	lines := strings.Split(sp.viewer.rendered, "\n")
	sp.viewer.cursorRow = len(lines) - 1
	sp.viewer.scrollOff = len(lines) - 1

	m.applyFold(sp, 2, true)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic after collapsing with cursor/scroll near the end: %v", r)
		}
	}()
	_ = m.View()

	got := m.splits[m.activeSplit].viewer
	newLines := strings.Split(got.rendered, "\n")
	if got.cursorRow >= len(newLines) {
		t.Errorf("cursorRow=%d out of range for %d rendered lines", got.cursorRow, len(newLines))
	}
	if got.scrollOff >= len(newLines) {
		t.Errorf("scrollOff=%d out of range for %d rendered lines", got.scrollOff, len(newLines))
	}
}

// TestHeadlessMouseClickOnHeadingTogglesFold covers the mouse path (#20):
// clicking anywhere on a heading's rendered line toggles its fold, the same
// as the sidebar's click-to-toggle convention.
func TestHeadlessMouseClickOnHeadingTogglesFold(t *testing.T) {
	m := setupTUI(t)
	m, _ = foldTestNote(t, m)
	sp := &m.splits[m.activeSplit]

	headingRow := -1
	for rl, raw := range sp.viewer.headingLines {
		if raw == 2 {
			headingRow = rl
		}
	}
	if headingRow < 0 {
		t.Fatalf("Section A heading not found in headingLines=%v", sp.viewer.headingLines)
	}

	// Reproduce the y-coordinate math handleMouseClick itself uses: content
	// starts at y=2, minus the sticky header, minus scrollOff (0 here). x
	// must land past the sidebar + gap or the click routes to the sidebar
	// instead of the main pane.
	y := headingRow + sp.viewer.headerLineCount + 1 + 2
	x := m.computeLayout().sidebarWidth + 5
	m.handleMouseClick(x, y)

	if !m.splits[m.activeSplit].viewer.folded[2] {
		t.Fatal("expected raw line 2 folded after clicking its heading")
	}

	m.handleMouseClick(x, y)
	if m.splits[m.activeSplit].viewer.folded[2] {
		t.Fatal("expected raw line 2 unfolded after clicking its heading again")
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

// TestHeadlessFencedCodeMatchesInlineCodeStyle guards against fenced ```
// code blocks rendering with glamour's default per-token chroma syntax
// highlighting (many distinct colors) while inline `code` spans render with
// a single flat color/background swatch. After the fix both must use the
// same flat styling — this asserts the fenced block collapses to exactly
// the SGR color codes inline code uses, not a rainbow of token colors.
func TestHeadlessFencedCodeMatchesInlineCodeStyle(t *testing.T) {
	m := setupTUI(t)
	n, err := m.vault.Create("Styled")
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	// Multiple distinct token types (keyword, string, comment, call) so
	// chroma highlighting, if still active, would emit several different
	// foreground colors instead of one.
	n.Body = "Some `inline code` here.\n\n```go\n" +
		"// a comment\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n```"
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
	span := sp.viewer.codeSpans[0]
	lines := strings.Split(sp.viewer.rendered, "\n")
	if span.endLine >= len(lines) {
		t.Fatalf("code span range %v out of bounds (%d rendered lines)", span, len(lines))
	}
	blockText := strings.Join(lines[span.startLine:span.endLine+1], "\n")

	colorSeqRe := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	blockColors := map[string]bool{}
	for _, c := range colorSeqRe.FindAllString(blockText, -1) {
		blockColors[c] = true
	}

	// A reset code ("\x1b[0m"/"\x1b[m") is not a color; every other distinct
	// sequence found must be the SAME single color pairing throughout the
	// block. Chroma syntax highlighting would produce several.
	nonReset := map[string]bool{}
	for c := range blockColors {
		if c != "\x1b[0m" && c != "\x1b[m" {
			nonReset[c] = true
		}
	}
	if len(nonReset) > 2 { // one foreground + one background code, at most
		t.Errorf("fenced code block uses %d distinct color codes (%v), want a flat swatch like inline code (<=2)", len(nonReset), nonReset)
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
	// Two notes were seeded, so items should be: Inbox, note1, note2, Projects,
	// ..., #templates, Tasks (the latter two are always-visible virtual rows).
	items := m.sidebar.items()
	if len(items) != len(vault.AllStates)+2+2 {
		t.Errorf("expected %d items after expand, got %d", len(vault.AllStates)+2+2, len(items))
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
	if len(items) != len(vault.AllStates)+2 {
		t.Errorf("expected %d items after collapse, got %d", len(vault.AllStates)+2, len(items))
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

// TestProjectDetailTaskRowsGroupedByNoteFiltered covers #19's core filter:
// only tasks belonging to the project's own notes appear, grouped by note
// (H2) in title order, tasks in file order — a note outside the project
// contributes nothing.
func TestProjectDetailTaskRowsGroupedByNoteFiltered(t *testing.T) {
	m := setupTUI(t)
	p, err := m.vault.Projects.Create("Homelab")
	if err != nil {
		t.Fatalf("Projects.Create: %v", err)
	}

	mkNote := func(title string, project, body string) *vault.Note {
		n, err := m.vault.Create(title)
		if err != nil {
			t.Fatalf("create %q: %v", title, err)
		}
		n.State = vault.StateProjects
		n.Project = project
		n.Body = body
		if err := m.vault.Save(n); err != nil {
			t.Fatalf("save %q: %v", title, err)
		}
		return n
	}

	noteB := mkNote("Zeta Note", "Homelab", "- [ ] task B")
	noteA := mkNote("Alpha Note", "Homelab", "- [x] task A")
	mkNote("Other Project Note", "Someplace Else", "- [ ] should not appear")

	rows := projectTaskRows([]*vault.Note{noteB, noteA})

	var got []string
	for _, r := range rows {
		switch {
		case r.fileNote != nil:
			got = append(got, "H2:"+r.fileNote.Title)
		case r.task != nil:
			done := "0"
			if r.task.done {
				done = "1"
			}
			got = append(got, "T:"+r.task.text+":"+done)
		}
	}
	want := []string{
		"H2:Alpha Note", "T:task A:1",
		"H2:Zeta Note", "T:task B:0",
	}
	if len(got) != len(want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d = %q, want %q (full: %v)", i, got[i], want[i], got)
		}
	}

	_ = p
}

// TestProjectDetailEmptyTasksNoPanic covers #19's empty-state constraint: a
// project with no tasks must render a clear "no tasks" line, not panic or
// leave a stray header.
func TestProjectDetailEmptyTasksNoPanic(t *testing.T) {
	m := setupTUI(t)
	p, err := m.vault.Projects.Create("Empty Homelab")
	if err != nil {
		t.Fatalf("Projects.Create: %v", err)
	}
	pd := newProjectDetailPane(p, nil)
	if len(pd.taskRows) != 0 {
		t.Fatalf("expected no task rows, got %v", pd.taskRows)
	}
	out := pd.render(60, 20)
	if !strings.Contains(xansi.Strip(out), "no tasks") {
		t.Errorf("expected empty-state message in render output, got:\n%s", xansi.Strip(out))
	}
	_ = m
}

// TestProjectDetailCursorSkipsHeadersAndClamps covers #19's cursor
// requirement: movement must skip note-header rows and clamp at both ends,
// never landing on a non-task row.
func TestProjectDetailCursorSkipsHeadersAndClamps(t *testing.T) {
	m := setupTUI(t)
	p, err := m.vault.Projects.Create("Homelab")
	if err != nil {
		t.Fatalf("Projects.Create: %v", err)
	}
	mkNote := func(title, body string) *vault.Note {
		n, err := m.vault.Create(title)
		if err != nil {
			t.Fatalf("create %q: %v", title, err)
		}
		n.State = vault.StateProjects
		n.Project = "Homelab"
		n.Body = body
		if err := m.vault.Save(n); err != nil {
			t.Fatalf("save %q: %v", title, err)
		}
		return n
	}
	mkNote("Alpha Note", "- [ ] only task in alpha")
	mkNote("Beta Note", "- [ ] first beta task\n- [ ] second beta task")

	allNotes, _ := m.vault.ListAll()
	pd := newProjectDetailPane(p, allNotes)

	// Cursor must start on a task row, not the first (header) row.
	if pd.taskRows[pd.taskCursorRow].task == nil {
		t.Fatalf("initial cursor row %d is not a task row: %v", pd.taskCursorRow, pd.taskRows[pd.taskCursorRow])
	}

	// Walk downward past the end: must clamp on the last task row, never a
	// header/spacer row.
	for i := 0; i < len(pd.taskRows)+2; i++ {
		pd = pd.moveTaskCursor(1)
		if pd.taskRows[pd.taskCursorRow].task == nil {
			t.Fatalf("cursor landed on non-task row %d after %d downward moves: %v", pd.taskCursorRow, i+1, pd.taskRows[pd.taskCursorRow])
		}
	}
	lastIdx := pd.taskCursorRow

	// Walk upward past the start: must clamp on the first task row.
	for i := 0; i < len(pd.taskRows)+2; i++ {
		pd = pd.moveTaskCursor(-1)
		if pd.taskRows[pd.taskCursorRow].task == nil {
			t.Fatalf("cursor landed on non-task row %d after %d upward moves: %v", pd.taskCursorRow, i+1, pd.taskRows[pd.taskCursorRow])
		}
	}
	firstIdx := pd.taskCursorRow
	if firstIdx == lastIdx {
		t.Fatalf("expected distinct first/last task rows, got both = %d", firstIdx)
	}
}

// TestHeadlessProjectDetailToggleTaskStampsDate covers #19's toggle
// requirement: Space on the task cursor rewrites the correct raw line in
// the correct note via the shared toggleCheckboxLine path (stamping the ✅
// date, same as the note viewer), and typing in the bridge does not move
// the task cursor.
func TestHeadlessProjectDetailToggleTaskStampsDate(t *testing.T) {
	m := setupTUI(t)
	p, err := m.vault.Projects.Create("Homelab")
	if err != nil {
		t.Fatalf("Projects.Create: %v", err)
	}
	n, err := m.vault.Create("Homelab Note")
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	n.State = vault.StateProjects
	n.Project = "Homelab"
	n.Body = "- [ ] toggle me"
	if err := m.vault.Save(n); err != nil {
		t.Fatalf("save note: %v", err)
	}
	m.index.Upsert(n)

	m.splits[m.activeSplit].projectDetail = newProjectDetailPane(p, []*vault.Note{n})
	m.splits[m.activeSplit].activeView = viewProjectDetail

	cursorBefore := m.splits[m.activeSplit].projectDetail.taskCursorRow

	// Typing into the bridge must not move the task cursor.
	m = step(t, m, key("e"), "e (start bridge entry)")
	m = step(t, m, key("j"), "j (typed into bridge, must not move task cursor)")
	m = step(t, m, key("k"), "k (typed into bridge, must not move task cursor)")
	if got := m.splits[m.activeSplit].projectDetail.taskCursorRow; got != cursorBefore {
		t.Errorf("task cursor moved while bridge had focus: got %d, want %d", got, cursorBefore)
	}
	if got := m.splits[m.activeSplit].projectDetail.bridgeInput; got != "jk" {
		t.Errorf("bridgeInput = %q, want %q", got, "jk")
	}
	m = step(t, m, key("esc"), "esc (leave bridge without submitting)")

	m = step(t, m, key(" "), "space (toggle task under cursor)")

	reloaded, err := m.vault.FindByTitle("Homelab Note")
	if err != nil {
		t.Fatalf("FindByTitle: %v", err)
	}
	today := timeNow().Format("2006-01-02")
	want := "- [x] toggle me ✅ " + today
	if strings.TrimSpace(reloaded.Body) != want {
		t.Errorf("body after toggle = %q, want %q", strings.TrimSpace(reloaded.Body), want)
	}

	row := m.splits[m.activeSplit].projectDetail.taskRows[m.splits[m.activeSplit].projectDetail.taskCursorRow]
	if row.task == nil || !row.task.done {
		t.Errorf("expected in-pane task row to reflect done=true after toggle, got %v", row.task)
	}
}

// TestHeadlessProjectDetailEnterJumpsToTaskSourceNote covers #19's
// jump-to-note requirement: Enter on the task cursor opens the task's
// source note, the same mechanism the :tasks overview uses.
func TestHeadlessProjectDetailEnterJumpsToTaskSourceNote(t *testing.T) {
	m := setupTUI(t)
	p, err := m.vault.Projects.Create("Homelab")
	if err != nil {
		t.Fatalf("Projects.Create: %v", err)
	}
	n, err := m.vault.Create("Jump Target")
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	n.State = vault.StateProjects
	n.Project = "Homelab"
	n.Body = "- [ ] find me"
	if err := m.vault.Save(n); err != nil {
		t.Fatalf("save note: %v", err)
	}
	m.index.Upsert(n)
	m.titleSet[strings.ToLower(n.Title)] = true

	m.splits[m.activeSplit].projectDetail = newProjectDetailPane(p, []*vault.Note{n})
	m.splits[m.activeSplit].activeView = viewProjectDetail

	m = step(t, m, key("enter"), "enter (jump to task's source note)")

	got := m.splits[m.activeSplit].viewer.note
	if got == nil || got.Title != "Jump Target" {
		t.Errorf("expected Enter to open Jump Target, got %v", got)
	}
	if m.splits[m.activeSplit].activeView != viewNote {
		t.Errorf("expected activeView=viewNote after jump, got %v", m.splits[m.activeSplit].activeView)
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

// TestHeadlessDeleteThenUndoLeavesTrashEmpty covers #1's most important
// trap, called out explicitly in its qualification: undoing a :delete must
// not just recreate the note (TestHeadlessDeleteThenUndo above) — it must
// also remove the trashed copy and its sidecar entry, or the note ends up
// existing twice, with the orphaned trash copy later surfacing in :trash as
// a ghost of a note that's already back.
func TestHeadlessDeleteThenUndoLeavesTrashEmpty(t *testing.T) {
	m := setupTUI(t)
	n, err := m.vault.Create("Doomed Too")
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	if err := m.vault.Save(n); err != nil {
		t.Fatalf("save note: %v", err)
	}
	m.index.Upsert(n)
	m.titleSet[strings.ToLower(n.Title)] = true

	m.cmdDelete([]string{"Doomed Too"})
	entries, _ := m.vault.ListTrash()
	if len(entries) != 1 {
		t.Fatalf("expected 1 trash entry after delete, got %d", len(entries))
	}
	trashPath := entries[0].TrashPath

	m.handleUndo()

	entries, _ = m.vault.ListTrash()
	if len(entries) != 0 {
		t.Fatalf("expected trash empty after undo, got %d entries: %+v", len(entries), entries)
	}
	if _, err := os.Stat(trashPath); !os.IsNotExist(err) {
		t.Errorf("expected the trash file removed after undo, stat err = %v", err)
	}
}

// TestHeadlessDeleteUndoRedoReTrashesExactlyOnce covers the "Nachtrag" in
// #1's qualification: a generic redo just calls Save, which would recreate
// the note instead of moving it back to trash — redo of an undone delete
// must go through vault.Trash again, landing the file in trash exactly
// once with exactly one sidecar entry, not drift the file/sidecar apart.
func TestHeadlessDeleteUndoRedoReTrashesExactlyOnce(t *testing.T) {
	m := setupTUI(t)
	n, err := m.vault.Create("Redo Target")
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	if err := m.vault.Save(n); err != nil {
		t.Fatalf("save note: %v", err)
	}
	m.index.Upsert(n)
	m.titleSet[strings.ToLower(n.Title)] = true
	origPath := n.Path

	m.cmdDelete([]string{"Redo Target"})
	m.handleUndo()
	if _, err := os.Stat(origPath); err != nil {
		t.Fatalf("expected file restored after undo: %v", err)
	}

	m.handleRedo()

	if _, err := os.Stat(origPath); !os.IsNotExist(err) {
		t.Fatalf("expected file gone from orig path after redo, stat err = %v", err)
	}
	entries, _ := m.vault.ListTrash()
	if len(entries) != 1 {
		t.Fatalf("expected exactly 1 trash entry after redo, got %d: %+v", len(entries), entries)
	}
	if _, err := os.Stat(entries[0].TrashPath); err != nil {
		t.Errorf("expected the trash file to exist at the recorded path: %v", err)
	}
	if m.titleSet[strings.ToLower("Redo Target")] {
		t.Error("expected titleSet entry removed again after redo")
	}
}

// TestHeadlessEscSavesEditorDraft guards against the editor silently
// discarding an in-progress edit on Esc: leaving the editor by any route must
// persist the draft (and register an undo step), never lose it.
func TestHeadlessEscSavesEditorDraft(t *testing.T) {
	m := setupTUI(t)
	m = step(t, m, key("enter"), "enter (open note)")
	m = step(t, m, key("e"), "e (open editor)")
	sp := m.splits[m.activeSplit]
	if sp.activeView != viewEdit {
		t.Fatal("expected viewEdit")
	}
	title := sp.editor.note.Title

	m = typeString(t, m, "unsaved change")
	stackLen := len(m.undoStack)

	m = step(t, m, key("esc"), "esc (exit editor without explicit save)")
	sp = m.splits[m.activeSplit]
	if sp.activeView == viewEdit {
		t.Fatal("expected editor to close on Esc")
	}

	n, err := m.vault.FindByTitle(title)
	if err != nil {
		t.Fatalf("FindByTitle: %v", err)
	}
	if !strings.Contains(n.Body, "unsaved change") {
		t.Errorf("body on disk = %q, want it to contain the typed text", n.Body)
	}
	if len(m.undoStack) != stackLen+1 {
		t.Errorf("undoStack len = %d, want %d (Esc-save should push an undo record)", len(m.undoStack), stackLen+1)
	}

	m.handleUndo()
	restored, err := m.vault.FindByTitle(title)
	if err != nil {
		t.Fatalf("FindByTitle after undo: %v", err)
	}
	if strings.Contains(restored.Body, "unsaved change") {
		t.Error("expected undo to revert the Esc-saved change")
	}
}

// TestHeadlessEscOnCleanEditorIsNoop guards against the Esc-saves fix
// (TestHeadlessEscSavesEditorDraft) spamming the undo stack when nothing
// changed — opening and immediately closing the editor must not save or
// push an undo record.
func TestHeadlessEscOnCleanEditorIsNoop(t *testing.T) {
	m := setupTUI(t)
	m = step(t, m, key("enter"), "enter (open note)")
	m = step(t, m, key("e"), "e (open editor)")
	stackLen := len(m.undoStack)

	m = step(t, m, key("esc"), "esc (exit editor with no changes)")
	sp := m.splits[m.activeSplit]
	if sp.activeView == viewEdit {
		t.Fatal("expected editor to close on Esc")
	}
	if len(m.undoStack) != stackLen {
		t.Errorf("undoStack len = %d, want unchanged %d (clean Esc must be a no-op save)", len(m.undoStack), stackLen)
	}
}

// TestHeadlessViewEditViewRoundTripPreservesRawLine covers #22: opening the
// editor from a cursor position deep in a wrapped note must start the
// textarea at that same raw line (not line 0), and saving back out must
// return the viewer cursor to that line (not jump to the top).
func TestHeadlessViewEditViewRoundTripPreservesRawLine(t *testing.T) {
	m := setupTUI(t)
	n, err := m.vault.Create("Wrap Round Trip")
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	longPara := strings.Repeat("supercalifragilistic ", 30)
	n.Body = "# Heading\n\n" + longPara + "\n\n- [ ] a task\n"
	if err := m.vault.Save(n); err != nil {
		t.Fatalf("save note: %v", err)
	}
	m.index.Upsert(n)
	m.titleSet[strings.ToLower(n.Title)] = true

	m.openOrCreateNote(n.Title)
	sp := &m.splits[m.activeSplit]
	if sp.activeView != viewNote || sp.viewer.note == nil {
		t.Fatalf("expected note open, activeView=%v", sp.activeView)
	}

	checkboxRendered, ok := firstKey(sp.viewer.checkboxLines)
	if !ok {
		t.Fatalf("expected a checkbox line in the rendered output")
	}
	sp.viewer.cursorRow = checkboxRendered
	wantRaw := sp.viewer.rawLineAt(checkboxRendered)
	if wantRaw == 0 {
		t.Fatalf("test setup: checkbox's raw line resolved to 0, want a nonzero line deep in the note (test wouldn't distinguish from the top-of-note bug)")
	}

	m = step(t, m, key("e"), "e (enter edit mode at the checkbox's position)")
	sp = &m.splits[m.activeSplit]
	if sp.activeView != viewEdit {
		t.Fatalf("expected viewEdit")
	}
	if got := sp.editor.ta.Line(); got != wantRaw {
		t.Errorf("editor cursor line on entry = %d, want %d (the view cursor's raw line)", got, wantRaw)
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeyCtrlS}, "ctrl+s (save, return to view)")
	sp = &m.splits[m.activeSplit]
	if sp.activeView != viewNote {
		t.Fatalf("expected viewNote after save")
	}
	if got := sp.viewer.rawLineAt(sp.viewer.cursorRow); got != wantRaw {
		t.Errorf("after save, viewer cursor's raw line = %d, want %d (should resume where editing left off, not jump to top)", got, wantRaw)
	}
}

// TestHeadlessEditorSaveDoesNotJumpViewerCursorToTop is the minimal
// regression test for #22's literal complaint ("cursor jumps to top on
// save"): commitEditorDraft used to call viewer.withNote (which zeroes
// cursorRow) without restoring it afterward, unlike the checkbox-toggle
// path (applyCheckboxToggle), which always has.
func TestHeadlessEditorSaveDoesNotJumpViewerCursorToTop(t *testing.T) {
	m := setupTUI(t)
	n, err := m.vault.Create("Save No Jump")
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	n.Body = "line one\n\nline two\n\nline three\n\nline four\n\nline five"
	if err := m.vault.Save(n); err != nil {
		t.Fatalf("save note: %v", err)
	}
	m.index.Upsert(n)
	m.titleSet[strings.ToLower(n.Title)] = true

	m.openOrCreateNote(n.Title)
	sp := &m.splits[m.activeSplit]
	lines := strings.Split(sp.viewer.rendered, "\n")
	if len(lines) < 3 {
		t.Fatalf("expected several rendered lines, got %d", len(lines))
	}
	sp.viewer.cursorRow = len(lines) - 1 // deliberately not the top

	m = step(t, m, key("e"), "e (open editor)")
	m = step(t, m, tea.KeyMsg{Type: tea.KeyCtrlS}, "ctrl+s (save, no changes made)")

	sp = &m.splits[m.activeSplit]
	if sp.viewer.cursorRow == 0 {
		t.Errorf("viewer cursor jumped to top after save, want it to stay near the bottom")
	}
}

// TestHeadlessBackspaceDeletesEmptyAutoPair guards #21: bubbles/textarea's
// backspace deletes exactly one rune, so an auto-closed empty pair like
// "(|)" left an orphaned closer behind. Backspace must now delete both
// halves when the cursor sits directly between a matching pair.
func TestHeadlessBackspaceDeletesEmptyAutoPair(t *testing.T) {
	for _, open := range []rune{'(', '[', '`'} {
		t.Run(string(open), func(t *testing.T) {
			m := setupTUI(t)
			m = step(t, m, key("enter"), "open note")
			m = step(t, m, key("e"), "open editor")

			m = typeString(t, m, string(open))
			body := m.splits[m.activeSplit].editor.ta.Value()
			want := string(open) + string(autoPairs[open])
			if body != want {
				t.Fatalf("after typing %q, body = %q, want %q", string(open), body, want)
			}

			m = step(t, m, key("backspace"), "backspace")
			body = m.splits[m.activeSplit].editor.ta.Value()
			if body != "" {
				t.Errorf("after backspace on empty pair, body = %q, want empty", body)
			}
		})
	}
}

// TestHeadlessBackspaceOnNonEmptyPairDeletesOnlyContent guards the "only
// delete the pair when adjacent" trap in #21: "(x|)" + backspace must delete
// just "x", leaving the pair intact — not scan past content to find a closer.
func TestHeadlessBackspaceOnNonEmptyPairDeletesOnlyContent(t *testing.T) {
	for _, open := range []rune{'(', '[', '`'} {
		t.Run(string(open), func(t *testing.T) {
			m := setupTUI(t)
			m = step(t, m, key("enter"), "open note")
			m = step(t, m, key("e"), "open editor")

			m = typeString(t, m, string(open))
			m = typeString(t, m, "x")
			m = step(t, m, key("backspace"), "backspace")

			body := m.splits[m.activeSplit].editor.ta.Value()
			want := string(open) + string(autoPairs[open])
			if body != want {
				t.Errorf("body = %q, want %q (only the content deleted, pair intact)", body, want)
			}
		})
	}
}

// TestHeadlessBackspaceThenReopenNoDoubleClose is the exact regression
// reported in #21: typing "(", backspace, typing "(" again must produce
// "()", not "())" — a stale orphaned closer from before the fix would
// combine with the fresh autoclose to leave an extra ")".
func TestHeadlessBackspaceThenReopenNoDoubleClose(t *testing.T) {
	m := setupTUI(t)
	m = step(t, m, key("enter"), "open note")
	m = step(t, m, key("e"), "open editor")

	m = typeString(t, m, "(")
	m = step(t, m, key("backspace"), "backspace")
	m = typeString(t, m, "(")

	body := m.splits[m.activeSplit].editor.ta.Value()
	if body != "()" {
		t.Errorf("body = %q, want %q", body, "()")
	}
}

// TestHeadlessBackspaceClosingBracketPairDismissesLinkSuggest guards the
// link-autosuggest interaction from #21: deleting a "[" via the new
// pair-backspace must refresh (and here, close) the link suggestion
// dropdown, not leave it stale.
func TestHeadlessBackspaceClosingBracketPairDismissesLinkSuggest(t *testing.T) {
	m := setupTUI(t)
	m = step(t, m, key("enter"), "open note")
	m = step(t, m, key("e"), "open editor")

	m = typeString(t, m, "[[")
	if !m.splits[m.activeSplit].editor.linkSuggestActive {
		t.Fatal("expected link suggest active after typing [[")
	}

	m = step(t, m, key("backspace"), "backspace")
	if m.splits[m.activeSplit].editor.linkSuggestActive {
		t.Error("expected link suggest inactive after backspace closed the outer [ pair")
	}
}

// TestHeadlessLineOpsYankAndPaste covers #10's yank/paste: yanking must not
// mutate the buffer, and pasting the register below the cursor line must
// duplicate it.
func TestHeadlessLineOpsYankAndPaste(t *testing.T) {
	m := setupTUI(t)
	m = step(t, m, key("enter"), "open note")
	m = step(t, m, key("e"), "open editor")

	m = typeString(t, m, "first line\nsecond line") // cursor ends on "second line"

	m = step(t, m, tea.KeyMsg{Type: tea.KeyCtrlL}, "ctrl+l (line-op leader)")
	m = step(t, m, key("y"), "y (yank current line)")
	if got := m.splits[m.activeSplit].editor.lineRegister; got != "second line" {
		t.Fatalf("lineRegister = %q, want %q", got, "second line")
	}
	if got := m.splits[m.activeSplit].editor.ta.Value(); got != "first line\nsecond line" {
		t.Fatalf("yank must not mutate the buffer, got %q", got)
	}
	if got := m.statusMsg; got != "line yanked" {
		t.Errorf("statusMsg = %q, want %q", got, "line yanked")
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeyCtrlL}, "ctrl+l (line-op leader)")
	m = step(t, m, key("p"), "p (paste below cursor)")
	want := "first line\nsecond line\nsecond line"
	if got := m.splits[m.activeSplit].editor.ta.Value(); got != want {
		t.Fatalf("body after paste = %q, want %q", got, want)
	}
}

// TestHeadlessLineOpsDeleteThenUndo covers #10's delete: the line must
// leave the buffer immediately, and — because the mutation goes through the
// same textarea value the normal save path commits — Ctrl+Z must recover it
// like any other edit.
func TestHeadlessLineOpsDeleteThenUndo(t *testing.T) {
	m := setupTUI(t)
	n, err := m.vault.Create("Delete Then Undo")
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	n.Body = "keep me\ndelete me"
	if err := m.vault.Save(n); err != nil {
		t.Fatalf("save note: %v", err)
	}
	m.index.Upsert(n)
	m.titleSet[strings.ToLower(n.Title)] = true
	title := n.Title

	m.openOrCreateNote(title)
	m = step(t, m, key("e"), "open editor")

	// Move to the second line ("delete me") — the editor opens at line 0.
	m = step(t, m, tea.KeyMsg{Type: tea.KeyDown}, "down (to second line)")

	m = step(t, m, tea.KeyMsg{Type: tea.KeyCtrlL}, "ctrl+l (line-op leader)")
	m = step(t, m, key("d"), "d (delete current line)")
	if got := m.splits[m.activeSplit].editor.ta.Value(); got != "keep me" {
		t.Fatalf("body after delete = %q, want %q", got, "keep me")
	}
	if got := m.splits[m.activeSplit].editor.lineRegister; got != "delete me" {
		t.Errorf("lineRegister after delete = %q, want %q", got, "delete me")
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeyCtrlS}, "ctrl+s (save)")
	saved, err := m.vault.FindByTitle(title)
	if err != nil {
		t.Fatalf("FindByTitle: %v", err)
	}
	if strings.Contains(saved.Body, "delete me") {
		t.Fatalf("saved body still contains the deleted line: %q", saved.Body)
	}

	m.handleUndo()
	restored, err := m.vault.FindByTitle(title)
	if err != nil {
		t.Fatalf("FindByTitle after undo: %v", err)
	}
	if !strings.Contains(restored.Body, "delete me") {
		t.Errorf("expected undo to restore the deleted line, body = %q", restored.Body)
	}
}

// TestHeadlessLineOpsDeleteLastLineClampsCursor covers #10's explicit
// out-of-range trap: deleting the only/last line must not leave the cursor
// past the end of a now-shorter (or empty) buffer.
func TestHeadlessLineOpsDeleteLastLineClampsCursor(t *testing.T) {
	m := setupTUI(t)
	m = step(t, m, key("enter"), "open note")
	m = step(t, m, key("e"), "open editor")
	m = typeString(t, m, "only line")

	m = step(t, m, tea.KeyMsg{Type: tea.KeyCtrlL}, "ctrl+l (line-op leader)")
	m = step(t, m, key("d"), "d (delete the only line)")

	sp := m.splits[m.activeSplit]
	if got := sp.editor.ta.Value(); got != "" {
		t.Errorf("body after deleting the only line = %q, want empty", got)
	}
	if line := sp.editor.ta.Line(); line != 0 {
		t.Errorf("cursor line after deleting the only line = %d, want 0", line)
	}
}

// TestHeadlessLineOpsPasteWithEmptyRegisterIsNoop covers #10's explicit
// empty-register case: paste before any yank/delete must not touch the
// buffer.
func TestHeadlessLineOpsPasteWithEmptyRegisterIsNoop(t *testing.T) {
	m := setupTUI(t)
	m = step(t, m, key("enter"), "open note")
	m = step(t, m, key("e"), "open editor")
	m = typeString(t, m, "only line")

	before := m.splits[m.activeSplit].editor.ta.Value()
	m = step(t, m, tea.KeyMsg{Type: tea.KeyCtrlL}, "ctrl+l (line-op leader)")
	m = step(t, m, key("p"), "p (paste, empty register)")
	if got := m.splits[m.activeSplit].editor.ta.Value(); got != before {
		t.Errorf("paste with empty register mutated the buffer: got %q, want unchanged %q", got, before)
	}
	if got := m.statusMsg; got != "nothing to paste" {
		t.Errorf("statusMsg = %q, want %q", got, "nothing to paste")
	}
}

// TestHeadlessLineOpsAbortOnUnrecognizedKeyDiscardsIt covers #10's abort
// case: any key other than y/d/p after Ctrl+L cancels the chord and is
// discarded — not typed into the buffer. Esc specifically must abort the
// *chord*, not the editor: the outer key switch's own "esc" case (cancel
// the whole editor) intercepts before updateBody ever runs, so the chord
// must be handled earlier in update() or "Ctrl+L then Esc" would silently
// close the editor instead of just cancelling the pending operation.
func TestHeadlessLineOpsAbortOnUnrecognizedKeyDiscardsIt(t *testing.T) {
	m := setupTUI(t)
	m = step(t, m, key("enter"), "open note")
	m = step(t, m, key("e"), "open editor")
	m = typeString(t, m, "hello")

	before := m.splits[m.activeSplit].editor.ta.Value()
	m = step(t, m, tea.KeyMsg{Type: tea.KeyCtrlL}, "ctrl+l (line-op leader)")
	if !m.splits[m.activeSplit].editor.lineOpPending {
		t.Fatal("expected lineOpPending after Ctrl+L")
	}
	m = step(t, m, key("a"), "a (not y/d/p — should abort and be discarded)")
	if m.splits[m.activeSplit].editor.lineOpPending {
		t.Error("expected lineOpPending cleared after any key")
	}
	if got := m.splits[m.activeSplit].editor.ta.Value(); got != before {
		t.Errorf("aborted chord inserted text: got %q, want unchanged %q", got, before)
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeyCtrlL}, "ctrl+l (line-op leader)")
	m = step(t, m, key("esc"), "esc (abort the chord, not the editor)")
	if m.splits[m.activeSplit].editor.lineOpPending {
		t.Error("expected lineOpPending cleared after esc")
	}
	if m.splits[m.activeSplit].activeView != viewEdit {
		t.Errorf("expected editor to stay open — esc during a pending chord should abort the chord, not cancel the editor, got activeView=%v", m.splits[m.activeSplit].activeView)
	}
}

// TestHeadlessLineOpsChordOnlyInBody covers #10's scope constraint: the
// Ctrl+L chord must not activate in the editor's header fields (title
// state/tags/project) or reach the note viewer.
func TestHeadlessLineOpsChordOnlyInBody(t *testing.T) {
	m := setupTUI(t)
	m = step(t, m, key("enter"), "open note")
	m = step(t, m, key("e"), "open editor")
	m = step(t, m, key("shift+tab"), "shift+tab (body → project field)")
	if m.splits[m.activeSplit].editor.focused != fldProject {
		t.Fatalf("expected focus on fldProject, got %v", m.splits[m.activeSplit].editor.focused)
	}

	m = step(t, m, tea.KeyMsg{Type: tea.KeyCtrlL}, "ctrl+l while a header field is focused")
	if m.splits[m.activeSplit].editor.lineOpPending {
		t.Error("expected Ctrl+L to be a no-op outside the body field")
	}
}

// TestHeadlessEditorAssignsProject guards issue #25: assigning a project via
// the editor's Project field must do everything :add project (cmdProject)
// does — force State to StateProjects, record attach history, and reveal the
// note in the sidebar's project tree — not just write the Project string.
func TestHeadlessEditorAssignsProject(t *testing.T) {
	m := setupTUI(t)
	if _, err := m.vault.Projects.Create("Homelab"); err != nil {
		t.Fatalf("Projects.Create: %v", err)
	}

	m = step(t, m, key("enter"), "enter (open note)")
	n := m.splits[m.activeSplit].viewer.note
	if n.State == vault.StateProjects {
		t.Fatal("test setup: note should not start in StateProjects")
	}

	m = step(t, m, key("e"), "e (open editor)")
	m = step(t, m, key("shift+tab"), "shift+tab (body → project field)")
	if m.splits[m.activeSplit].editor.focused != fldProject {
		t.Fatalf("expected focus on fldProject, got %v", m.splits[m.activeSplit].editor.focused)
	}
	m = typeString(t, m, "Homelab")
	m = step(t, m, tea.KeyMsg{Type: tea.KeyCtrlS}, "ctrl+s (save)")

	saved, err := m.vault.FindByTitle(n.Title)
	if err != nil {
		t.Fatalf("FindByTitle: %v", err)
	}
	if saved.State != vault.StateProjects {
		t.Errorf("State = %q, want %q", saved.State, vault.StateProjects)
	}
	if saved.Project != "Homelab" {
		t.Errorf("Project = %q, want %q", saved.Project, "Homelab")
	}

	p, ok := m.vault.Projects.Get("Homelab")
	if !ok {
		t.Fatal("expected Homelab project to exist")
	}
	found := false
	for _, h := range p.History {
		if h.Kind == vault.HistoryKindAttached && h.NoteID == saved.ID {
			found = true
		}
	}
	if !found {
		t.Error("expected an attached history entry for the note")
	}

	if !m.sidebar.expanded[vault.StateProjects] {
		t.Error("expected Projects section expanded to reveal the note")
	}
	if !m.sidebar.expandedProjects["Homelab"] {
		t.Error("expected Homelab project folder expanded to reveal the note")
	}
	revealed := false
	for _, item := range m.sidebar.items() {
		if item.isProjectNote && item.note != nil && item.note.ID == saved.ID {
			revealed = true
		}
	}
	if !revealed {
		t.Error("expected the note to appear among Homelab's revealed sidebar items")
	}
}

// TestHeadlessEditorClearsProject guards the detach side of #25: clearing the
// Project field on a project note in the editor must record a detach and
// return the note to Inbox, not leave it orphaned in StateProjects with an
// empty Project.
func TestHeadlessEditorClearsProject(t *testing.T) {
	m := setupTUI(t)
	if _, err := m.vault.Projects.Create("Homelab"); err != nil {
		t.Fatalf("Projects.Create: %v", err)
	}
	n, err := m.vault.Create("Router notes")
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	n.Project = "Homelab"
	if err := m.vault.SetState(n, vault.StateProjects); err != nil {
		t.Fatalf("SetState: %v", err)
	}
	m.index.Upsert(n)
	m.titleSet[strings.ToLower(n.Title)] = true

	sp := &m.splits[m.activeSplit]
	sp.viewer = sp.viewer.withNote(n)
	sp.activeView = viewNote
	m.activePane = paneMain

	m = step(t, m, key("e"), "e (open editor)")
	m = step(t, m, key("shift+tab"), "shift+tab (body → project field)")
	// Clear the pre-filled "Homelab" value.
	for range "Homelab" {
		m = step(t, m, key("backspace"), "backspace")
	}
	m = step(t, m, tea.KeyMsg{Type: tea.KeyCtrlS}, "ctrl+s (save)")

	saved, err := m.vault.FindByTitle("Router notes")
	if err != nil {
		t.Fatalf("FindByTitle: %v", err)
	}
	if saved.Project != "" {
		t.Errorf("Project = %q, want empty", saved.Project)
	}
	if saved.State != vault.StateInbox {
		t.Errorf("State = %q, want %q", saved.State, vault.StateInbox)
	}

	p, _ := m.vault.Projects.Get("Homelab")
	found := false
	for _, h := range p.History {
		if h.Kind == vault.HistoryKindDetached && h.NoteID == saved.ID {
			found = true
		}
	}
	if !found {
		t.Error("expected a detached history entry for the note")
	}
}

// TestHeadlessEditorProjectAssignmentAtMaxLeavesDraftIntact guards the
// max-active-projects trap from #25's spec: EnsureProject is validated
// before anything is mutated, so a max-reached error must leave the editor
// open with the draft untouched rather than silently losing it.
func TestHeadlessEditorProjectAssignmentAtMaxLeavesDraftIntact(t *testing.T) {
	m := setupTUI(t)
	for i := 0; i < vault.MaxProjects; i++ {
		if _, err := m.vault.Projects.Create(string(rune('A' + i))); err != nil {
			t.Fatalf("Projects.Create: %v", err)
		}
	}

	m = step(t, m, key("enter"), "enter (open note)")
	n := m.splits[m.activeSplit].viewer.note
	origState := n.State

	m = step(t, m, key("e"), "e (open editor)")
	m = typeString(t, m, " extra text")
	m = step(t, m, key("shift+tab"), "shift+tab (body → project field)")
	m = typeString(t, m, "BrandNewProject")
	m = step(t, m, tea.KeyMsg{Type: tea.KeyCtrlS}, "ctrl+s (save)")

	sp := m.splits[m.activeSplit]
	if sp.activeView != viewEdit {
		t.Fatal("expected editor to stay open after a max-projects error")
	}
	if m.statusMsg == "" {
		t.Error("expected an error statusMsg")
	}

	onDisk, err := m.vault.FindByTitle(n.Title)
	if err != nil {
		t.Fatalf("FindByTitle: %v", err)
	}
	if strings.Contains(onDisk.Body, "extra text") {
		t.Error("expected the draft NOT to be saved to disk after a max-projects error")
	}
	if onDisk.State != origState {
		t.Errorf("State = %q, want unchanged %q", onDisk.State, origState)
	}
}

// TestEditorProjectSuggestPrefixMatch guards the Project field's autosuggest,
// which mirrors the palette's slotProject convention: case-insensitive
// PREFIX match only (not substring) over active project names.
func TestEditorProjectSuggestPrefixMatch(t *testing.T) {
	e := editPane{projectNames: []string{"Homelab", "Home Renovation", "Work Stuff"}}

	got := e.filterProjects("home")
	want := []string{"Homelab", "Home Renovation"}
	if len(got) != len(want) {
		t.Fatalf("filterProjects(%q) = %v, want %v", "home", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("filterProjects(%q)[%d] = %q, want %q", "home", i, got[i], want[i])
		}
	}

	// "stuff" is a substring of "Work Stuff" but not a prefix — must NOT match.
	if got := e.filterProjects("stuff"); len(got) != 0 {
		t.Errorf("filterProjects(%q) = %v, want no matches (prefix-only)", "stuff", got)
	}

	// Empty fragment lists everything, capped at linkSuggestMax.
	if got := e.filterProjects(""); len(got) != linkSuggestMax {
		t.Errorf("filterProjects(\"\") returned %d entries, want %d (capped)", len(got), linkSuggestMax)
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

// TestFitTooltipBarNeverWrapsToASecondLine is the unit-level counterpart to
// TestHeadlessViewNeverExceedsRequestedHeight: fitTooltipBar itself must
// never produce more than one rendered line, at any width, for a bar with
// far more chip text than a narrow terminal can hold.
func TestFitTooltipBarNeverWrapsToASecondLine(t *testing.T) {
	longBar := strings.Repeat("chip content ", 40)
	for _, w := range []int{200, 100, 60, 40, 20, 10, 1, 0} {
		out := fitTooltipBar(longBar, w)
		if strings.Count(out, "\n") != 0 {
			t.Errorf("fitTooltipBar(width=%d) produced %d newlines, want 0: %q", w, strings.Count(out, "\n"), out)
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

// openExportPopover opens a note via the note list, then runs :export
// through the palette, returning the resulting Model.
func openExportPopover(t *testing.T, m Model) Model {
	t.Helper()
	m = step(t, m, key("enter"), "enter (open note)")
	m = step(t, m, key(":"), "open palette")
	m = typeInPalette(t, m, "export")
	m = step(t, m, key("enter"), "submit :export")
	return m
}

// TestHeadlessExportPrefillsCurrentNoteFilename guards #23's autosuggest
// requirement: the path field must default to the open note's own filename
// (not a filesystem path, unlike :import's autosuggest), so Enter alone
// exports to the working directory under the note's own name.
func TestHeadlessExportPrefillsCurrentNoteFilename(t *testing.T) {
	m := setupTUI(t)
	m = openExportPopover(t, m)
	if !m.showExport {
		t.Fatalf("expected showExport=true after :export, statusMsg=%q", m.statusMsg)
	}
	n := m.splits[m.activeSplit].viewer.note
	want := vault.Filename(n.ID, n.Title)
	if got := m.exportView.pathInput.Value(); got != want {
		t.Errorf("path field = %q, want prefilled with %q", got, want)
	}
}

// TestHeadlessExportWritesByteIdenticalCopy guards the core behavior: export
// is a byte-for-byte copy of the note's on-disk file (frontmatter included,
// so it round-trips via :import), and the vault copy is never touched.
func TestHeadlessExportWritesByteIdenticalCopy(t *testing.T) {
	m := setupTUI(t)
	m = openExportPopover(t, m)
	n := m.splits[m.activeSplit].viewer.note
	original, err := os.ReadFile(n.Path)
	if err != nil {
		t.Fatalf("read original note file: %v", err)
	}

	outDir := t.TempDir()
	outPath := filepath.Join(outDir, "exported.md")

	// Clear the prefilled value and type the target path.
	for range m.exportView.pathInput.Value() {
		m = step(t, m, key("backspace"), "backspace")
	}
	m = typeString(t, m, outPath)
	m = step(t, m, key("tab"), "tab (-> confirm)")
	if m.exportView.focused != expFldConfirm {
		t.Fatalf("expected focus on expFldConfirm, got %v", m.exportView.focused)
	}
	m = step(t, m, key("enter"), "enter (confirm export)")

	if m.showExport {
		t.Fatalf("expected popover closed after successful export, errMsg=%q", m.exportView.errMsg)
	}
	written, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read exported file: %v", err)
	}
	if string(written) != string(original) {
		t.Errorf("exported bytes differ from source:\nexported=%q\nsource=  %q", written, original)
	}
	// Vault copy must be byte-identical to what it was before (untouched).
	afterExport, err := os.ReadFile(n.Path)
	if err != nil {
		t.Fatalf("re-read vault note: %v", err)
	}
	if string(afterExport) != string(original) {
		t.Error("vault note was modified by export")
	}
}

// TestHeadlessExportExistingTargetRequiresConfirm guards the overwrite-guard
// trap: a first Enter on an existing target must NOT write, only arm a
// pending confirmation; a second Enter on the same path then overwrites.
func TestHeadlessExportExistingTargetRequiresConfirm(t *testing.T) {
	m := setupTUI(t)
	m = openExportPopover(t, m)

	outDir := t.TempDir()
	outPath := filepath.Join(outDir, "exported.md")
	if err := os.WriteFile(outPath, []byte("pre-existing content"), 0o644); err != nil {
		t.Fatalf("seed existing target: %v", err)
	}

	for range m.exportView.pathInput.Value() {
		m = step(t, m, key("backspace"), "backspace")
	}
	m = typeString(t, m, outPath)
	m = step(t, m, key("tab"), "tab (-> confirm)")
	m = step(t, m, key("enter"), "enter (first confirm — should only arm overwrite)")

	if !m.showExport {
		t.Fatal("expected popover to stay open after the first confirm on an existing target")
	}
	if m.exportView.errMsg == "" {
		t.Error("expected an overwrite-warning errMsg after the first confirm")
	}
	stillOld, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(stillOld) != "pre-existing content" {
		t.Fatal("target was overwritten before the second confirm")
	}

	m = step(t, m, key("enter"), "enter (second confirm — should overwrite)")
	if m.showExport {
		t.Fatalf("expected popover closed after the second confirm, errMsg=%q", m.exportView.errMsg)
	}
	overwritten, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read target after overwrite: %v", err)
	}
	if string(overwritten) == "pre-existing content" {
		t.Error("expected the target to be overwritten after the second confirm")
	}
}

// TestHeadlessExportNonexistentDirectoryErrors guards against a panic and
// requires a clear error rather than implicitly creating directories.
func TestHeadlessExportNonexistentDirectoryErrors(t *testing.T) {
	m := setupTUI(t)
	m = openExportPopover(t, m)

	missing := filepath.Join(t.TempDir(), "does-not-exist", "out.md")
	for range m.exportView.pathInput.Value() {
		m = step(t, m, key("backspace"), "backspace")
	}
	m = typeString(t, m, missing)
	m = step(t, m, key("tab"), "tab (-> confirm)")
	m = step(t, m, key("enter"), "enter (confirm export)")

	if !m.showExport {
		t.Fatal("expected popover to stay open when the target directory doesn't exist")
	}
	if m.exportView.errMsg == "" {
		t.Error("expected an error message for a nonexistent directory")
	}
	if _, err := os.Stat(missing); err == nil {
		t.Error("expected no file to be created")
	}
}

// TestHeadlessExportDirectoryPathExportsUnderNoteFilename guards the "bare
// directory as the path" behavior: it exports into that directory under the
// note's own filename rather than erroring or requiring an explicit name.
func TestHeadlessExportDirectoryPathExportsUnderNoteFilename(t *testing.T) {
	m := setupTUI(t)
	m = openExportPopover(t, m)
	n := m.splits[m.activeSplit].viewer.note

	outDir := t.TempDir()
	for range m.exportView.pathInput.Value() {
		m = step(t, m, key("backspace"), "backspace")
	}
	m = typeString(t, m, outDir+string(os.PathSeparator))
	m = step(t, m, key("tab"), "tab (-> confirm)")
	m = step(t, m, key("enter"), "enter (confirm export)")

	if m.showExport {
		t.Fatalf("expected popover closed after successful export, errMsg=%q", m.exportView.errMsg)
	}
	wantPath := filepath.Join(outDir, vault.Filename(n.ID, n.Title))
	if _, err := os.Stat(wantPath); err != nil {
		t.Errorf("expected file at %s, stat error: %v", wantPath, err)
	}
}

// TestHeadlessExportNoNoteOpenShowsError guards against opening an empty
// prompt (or panicking) when :export is run with no note open.
func TestHeadlessExportNoNoteOpenShowsError(t *testing.T) {
	m := setupTUI(t)
	m = step(t, m, key(":"), "open palette")
	m = typeInPalette(t, m, "export")
	m = step(t, m, key("enter"), "submit :export with no note open")

	if m.showExport {
		t.Error("expected showExport=false when no note is open")
	}
	if m.statusMsg == "" {
		t.Error("expected an error statusMsg when no note is open")
	}
}

// --- Config: show/hide Tasks nav and Templates (issue #14) ---

// TestApplyConfigItemTogglesTasksNavVisibility covers the sidebar reacting
// live to the config toggle, and the cursor being clamped if it was sitting
// on a row that just disappeared.
func TestApplyConfigItemTogglesTasksNavVisibility(t *testing.T) {
	m := setupTUI(t)
	if !m.cfg.ShowTasksNav {
		t.Fatal("expected ShowTasksNav to default true")
	}

	items := m.sidebar.items()
	tasksIdx := -1
	for i, it := range items {
		if it.isTasks {
			tasksIdx = i
			break
		}
	}
	if tasksIdx < 0 {
		t.Fatalf("expected a Tasks row by default: %+v", items)
	}
	m.sidebar.cursor = tasksIdx // park the cursor on the row about to vanish

	m.applyConfigItem(cfgItemShowTasksNav, 1) // valueIdx 1 == "off"

	if m.cfg.ShowTasksNav {
		t.Error("expected cfg.ShowTasksNav = false after toggling off")
	}
	items = m.sidebar.items()
	for _, it := range items {
		if it.isTasks {
			t.Fatalf("Tasks row still present after toggling ShowTasksNav off: %+v", items)
		}
	}
	if m.sidebar.cursor >= len(items) {
		t.Errorf("cursor = %d not clamped after hiding a row (len=%d)", m.sidebar.cursor, len(items))
	}

	// Toggle back on: the row must return.
	m.applyConfigItem(cfgItemShowTasksNav, 0) // valueIdx 0 == "on"
	if !m.cfg.ShowTasksNav {
		t.Error("expected cfg.ShowTasksNav = true after toggling on")
	}
	found := false
	for _, it := range m.sidebar.items() {
		if it.isTasks {
			found = true
			break
		}
	}
	if !found {
		t.Error("Tasks row did not return after toggling ShowTasksNav back on")
	}
}

// TestApplyConfigItemTogglesTemplatesNavIndependently covers the other half
// of #14 — Templates visibility must be independent of the Tasks flag.
func TestApplyConfigItemTogglesTemplatesNavIndependently(t *testing.T) {
	m := setupTUI(t)

	m.applyConfigItem(cfgItemShowTemplatesNav, 1) // off

	items := m.sidebar.items()
	hasTemplates, hasTasks := false, false
	for _, it := range items {
		if it.isTemplates && it.isSection {
			hasTemplates = true
		}
		if it.isTasks {
			hasTasks = true
		}
	}
	if hasTemplates {
		t.Error("expected #templates row hidden after toggling ShowTemplatesNav off")
	}
	if !hasTasks {
		t.Error("Tasks row should remain visible — ShowTemplatesNav must not affect it")
	}
}

// TestDefaultConfigTrashRetentionDaysIs30 covers #1's config default.
func TestDefaultConfigTrashRetentionDaysIs30(t *testing.T) {
	cfg := defaultConfig()
	if cfg.TrashRetentionDays != 30 {
		t.Errorf("TrashRetentionDays = %d, want 30", cfg.TrashRetentionDays)
	}
}

// TestFillConfigDefaultsSetsRetentionForOldConfig covers #1's explicit
// backward-compatibility requirement: a config written before this field
// existed (zero value after YAML unmarshal) must default to 30, the same
// way every other pre-existing field in fillConfigDefaults already does.
func TestFillConfigDefaultsSetsRetentionForOldConfig(t *testing.T) {
	cfg := AppConfig{} // simulates an old config.yaml with no trash_retention_days key
	fillConfigDefaults(&cfg)
	if cfg.TrashRetentionDays != 30 {
		t.Errorf("TrashRetentionDays after fillConfigDefaults = %d, want 30", cfg.TrashRetentionDays)
	}
}

// TestApplyConfigItemChangesTrashRetentionDays covers the config pane's
// numeric field (a preset cycle, per #1's qualification — reusing the
// General section's existing fixed-choice UI rather than a new widget).
func TestApplyConfigItemChangesTrashRetentionDays(t *testing.T) {
	m := setupTUI(t)
	if m.cfg.TrashRetentionDays != 30 {
		t.Fatalf("test setup: expected default 30, got %d", m.cfg.TrashRetentionDays)
	}
	m.applyConfigItem(cfgItemTrashRetention, 0) // "7 days"
	if m.cfg.TrashRetentionDays != 7 {
		t.Errorf("TrashRetentionDays = %d, want 7", m.cfg.TrashRetentionDays)
	}
	m.applyConfigItem(cfgItemTrashRetention, 4) // "90 days"
	if m.cfg.TrashRetentionDays != 90 {
		t.Errorf("TrashRetentionDays = %d, want 90", m.cfg.TrashRetentionDays)
	}
}

// --- Trash view (issue #1) ---

func TestHeadlessTrashCommandListsAndRestores(t *testing.T) {
	m := setupTUI(t)
	n, err := m.vault.Create("Trashable")
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	if err := m.vault.Save(n); err != nil {
		t.Fatalf("save note: %v", err)
	}
	m.index.Upsert(n)
	m.titleSet[strings.ToLower(n.Title)] = true
	origPath := n.Path

	m.cmdDelete([]string{"Trashable"})

	m = step(t, m, key(":"), "colon (open palette)")
	m = typeInPalette(t, m, "trash")
	m = step(t, m, key("enter"), "enter (run :trash)")

	sp := m.splits[m.activeSplit]
	if sp.activeView != viewTrash {
		t.Fatalf("expected viewTrash, got %v", sp.activeView)
	}
	if len(sp.trashRows) != 1 || sp.trashRows[0].Title != "Trashable" {
		t.Fatalf("trashRows = %+v, want 1 entry titled Trashable", sp.trashRows)
	}

	m = step(t, m, key("enter"), "enter (restore)")

	sp = m.splits[m.activeSplit]
	if len(sp.trashRows) != 0 {
		t.Errorf("expected trashRows empty after restore, got %+v", sp.trashRows)
	}
	if _, err := os.Stat(origPath); err != nil {
		t.Errorf("expected file restored to orig path: %v", err)
	}
	if _, err := m.vault.FindByTitle("Trashable"); err != nil {
		t.Errorf("FindByTitle after restore: %v", err)
	}
}

// TestHeadlessTrashPermanentDeleteRequiresConfirm covers the footer-line
// confirm (not a modal): a single "d" must not delete anything yet, only a
// second "d" on the same row does, and it's irreversible (unlike :delete,
// no undo record — this note already survived one Ctrl+Z chance).
func TestHeadlessTrashPermanentDeleteRequiresConfirm(t *testing.T) {
	m := setupTUI(t)
	n, err := m.vault.Create("Gone For Good")
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	if err := m.vault.Save(n); err != nil {
		t.Fatalf("save note: %v", err)
	}
	m.index.Upsert(n)
	m.cmdDelete([]string{"Gone For Good"})
	m.openTrashView()

	trashPath := m.splits[m.activeSplit].trashRows[0].TrashPath

	m = step(t, m, key("d"), "d (request permanent-delete confirm)")
	sp := m.splits[m.activeSplit]
	if len(sp.trashRows) != 1 {
		t.Fatalf("expected the row to survive a single d, trashRows = %+v", sp.trashRows)
	}
	if sp.trashConfirmID == "" {
		t.Fatal("expected trashConfirmID set after the first d")
	}
	if _, err := os.Stat(trashPath); err != nil {
		t.Errorf("expected the trash file to still exist after a single d: %v", err)
	}

	m = step(t, m, key("d"), "d (confirm permanent delete)")
	sp = m.splits[m.activeSplit]
	if len(sp.trashRows) != 0 {
		t.Fatalf("expected the row gone after the second d, trashRows = %+v", sp.trashRows)
	}
	if _, err := os.Stat(trashPath); !os.IsNotExist(err) {
		t.Errorf("expected the trash file removed, stat err = %v", err)
	}
	entries, _ := m.vault.ListTrash()
	if len(entries) != 0 {
		t.Errorf("expected the sidecar entry removed too, got %+v", entries)
	}
}

// TestHeadlessTrashOtherKeyCancelsConfirm covers the confirm's cancel path:
// any key other than a repeated "d" on the same row clears the pending
// confirm without deleting anything.
func TestHeadlessTrashOtherKeyCancelsConfirm(t *testing.T) {
	m := setupTUI(t)
	n, err := m.vault.Create("Reprieve")
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	if err := m.vault.Save(n); err != nil {
		t.Fatalf("save note: %v", err)
	}
	m.index.Upsert(n)
	m.cmdDelete([]string{"Reprieve"})
	m.openTrashView()

	m = step(t, m, key("d"), "d (request confirm)")
	if m.splits[m.activeSplit].trashConfirmID == "" {
		t.Fatal("expected trashConfirmID set after d")
	}

	m = step(t, m, key("j"), "j (any other key cancels the confirm)")
	sp := m.splits[m.activeSplit]
	if sp.trashConfirmID != "" {
		t.Error("expected trashConfirmID cleared after a non-d key")
	}
	if len(sp.trashRows) != 1 {
		t.Errorf("expected the row untouched, trashRows = %+v", sp.trashRows)
	}
}

// TestHeadlessTrashEmptyStateNoPanic covers the empty-trash render path.
func TestHeadlessTrashEmptyStateNoPanic(t *testing.T) {
	m := setupTUI(t)
	m.openTrashView()
	sp := m.splits[m.activeSplit]
	if len(sp.trashRows) != 0 {
		t.Fatalf("expected empty trash, got %+v", sp.trashRows)
	}
	out := renderTrash(sp.trashRows, m.cfg.TrashRetentionDays, 60, 10, 0, 0, "", true)
	if !strings.Contains(xansi.Strip(out), "empty") {
		t.Errorf("expected an empty-state message, got:\n%s", xansi.Strip(out))
	}
}

// --- Task Overview (issue #13) ---

// TestBuildTaskOverviewRows covers the grouping/ordering rules: active
// projects in sidebar order, files sorted by title within each group, an
// "Unassigned" trailing group for non-project notes, and notes with no
// tasks (or projects with no task-bearing notes) excluded entirely.
func TestBuildTaskOverviewRows(t *testing.T) {
	m := setupTUI(t)

	// Two projects, created in an order that differs from alphabetical, to
	// prove grouping follows Projects.ListActive() order, not name sort.
	if _, err := m.vault.Projects.Create("Zeta"); err != nil {
		t.Fatalf("create project Zeta: %v", err)
	}
	if _, err := m.vault.Projects.Create("Alpha"); err != nil {
		t.Fatalf("create project Alpha: %v", err)
	}
	// A third project with no task-bearing notes must not appear at all.
	if _, err := m.vault.Projects.Create("Empty Project"); err != nil {
		t.Fatalf("create project Empty Project: %v", err)
	}

	mkNote := func(title string, state vault.NoteState, project, body string) *vault.Note {
		n, err := m.vault.Create(title)
		if err != nil {
			t.Fatalf("create %q: %v", title, err)
		}
		n.State = state
		n.Project = project
		n.Body = body
		if err := m.vault.Save(n); err != nil {
			t.Fatalf("save %q: %v", title, err)
		}
		return n
	}

	// Zeta: two notes, out-of-title-order creation to prove file sort.
	mkNote("Zeta Second File", vault.StateProjects, "Zeta", "- [ ] Zeta task B")
	mkNote("Zeta First File", vault.StateProjects, "Zeta", "- [ ] Zeta task A")
	// Alpha: one note.
	mkNote("Alpha File", vault.StateProjects, "Alpha", "- [x] Alpha task")
	// Empty Project: a note assigned to it, but with no task lines — the
	// project must not appear in the overview at all.
	mkNote("Alpha File Untasked", vault.StateProjects, "Empty Project", "just prose, no tasks")
	// Unassigned: an Inbox note with a task.
	mkNote("Loose Inbox Note", vault.StateInbox, "", "- [ ] Loose task")
	// A note with zero tasks anywhere must not create any row.
	mkNote("Totally Task-Free", vault.StateInbox, "", "no tasks here either")

	rows := buildTaskOverviewRows(m.vault)

	var got []string
	for _, r := range rows {
		switch {
		case r.projectHeader != "":
			got = append(got, "H1:"+r.projectHeader)
		case r.fileNote != nil:
			got = append(got, "H2:"+r.fileNote.Title)
		case r.task != nil:
			got = append(got, "T:"+r.task.text)
		}
	}

	want := []string{
		"H1:Zeta",
		"H2:Zeta First File", "T:Zeta task A",
		"H2:Zeta Second File", "T:Zeta task B",
		"H1:Alpha",
		"H2:Alpha File", "T:Alpha task",
		"H1:Unassigned",
		"H2:Loose Inbox Note", "T:Loose task",
	}
	if len(got) != len(want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d = %q, want %q (full: %v)", i, got[i], want[i], got)
		}
	}
}

func TestHeadlessTasksCommandOpensOverview(t *testing.T) {
	m := setupTUI(t)
	n, err := m.vault.Create("With Tasks")
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	n.Body = "- [ ] Do the thing"
	if err := m.vault.Save(n); err != nil {
		t.Fatalf("save note: %v", err)
	}
	m.index.Upsert(n)

	m = step(t, m, key(":"), "colon (open palette)")
	m = typeInPalette(t, m, "tasks")
	m = step(t, m, key("enter"), "enter (run :tasks)")

	sp := m.splits[m.activeSplit]
	if sp.activeView != viewTasksOverview {
		t.Fatalf("expected viewTasksOverview, got %v", sp.activeView)
	}
	if len(sp.taskRows) == 0 {
		t.Fatal("expected at least one task row after :tasks")
	}
	if !strings.Contains(m.renderBreadcrumb(), "Tasks") {
		t.Errorf("breadcrumb = %q, want it to mention Tasks", m.renderBreadcrumb())
	}
}

// TestHeadlessTasksOverviewCursorOpensSourceNote covers the core v1
// interaction: moving the cursor onto a task row and pressing Enter opens
// that task's source note, leaving the overview.
func TestHeadlessTasksOverviewCursorOpensSourceNote(t *testing.T) {
	m := setupTUI(t)
	n, err := m.vault.Create("Task Source")
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	n.Body = "- [ ] Do the thing"
	if err := m.vault.Save(n); err != nil {
		t.Fatalf("save note: %v", err)
	}
	m.index.Upsert(n)
	m.titleSet[strings.ToLower(n.Title)] = true

	m.openTasksOverview()
	sp := &m.splits[m.activeSplit]

	taskRow := -1
	for i, r := range sp.taskRows {
		if r.task != nil {
			taskRow = i
			break
		}
	}
	if taskRow < 0 {
		t.Fatalf("no task row found: %v", sp.taskRows)
	}
	sp.taskCursorRow = taskRow

	m = step(t, m, key("enter"), "enter (open task's source note)")

	got := m.splits[m.activeSplit].viewer.note
	if got == nil || got.Title != "Task Source" {
		t.Errorf("expected Enter to open Task Source, got %v", got)
	}
	if m.splits[m.activeSplit].activeView != viewNote {
		t.Errorf("expected activeView=viewNote after opening, got %v", m.splits[m.activeSplit].activeView)
	}
}

func TestHeadlessTasksOverviewEscReturnsToList(t *testing.T) {
	m := setupTUI(t)
	m.openTasksOverview()
	m = step(t, m, key("esc"), "esc (close task overview)")
	if m.splits[m.activeSplit].activeView != viewList {
		t.Errorf("expected viewList after Esc, got %v", m.splits[m.activeSplit].activeView)
	}
}

// TestHeadlessSidebarTasksRowKeyboard covers navigating the sidebar cursor
// to the virtual Tasks row and activating it with Enter.
func TestHeadlessSidebarTasksRowKeyboard(t *testing.T) {
	m := setupTUI(t)
	n, err := m.vault.Create("Has A Task")
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	n.Body = "- [ ] Something"
	if err := m.vault.Save(n); err != nil {
		t.Fatalf("save note: %v", err)
	}
	m.index.Upsert(n)

	m = step(t, m, key("tab"), "tab (-> sidebar)")
	if m.activePane != paneSidebar {
		t.Fatal("expected focus on sidebar")
	}

	items := m.sidebar.items()
	tasksIdx := -1
	for i, it := range items {
		if it.isTasks {
			tasksIdx = i
			break
		}
	}
	if tasksIdx < 0 {
		t.Fatalf("no Tasks row found in sidebar items: %+v", items)
	}
	m.sidebar.cursor = tasksIdx

	m = step(t, m, key("enter"), "enter (activate Tasks row)")

	if m.splits[m.activeSplit].activeView != viewTasksOverview {
		t.Errorf("expected viewTasksOverview, got %v", m.splits[m.activeSplit].activeView)
	}
	if m.activePane != paneMain {
		t.Error("expected focus to move to main pane")
	}
	if !m.sidebar.tasksActive {
		t.Error("expected sidebar.tasksActive=true")
	}
}

// TestHeadlessSidebarTasksRowDoesNotCorruptInboxState guards against the
// zero-value trap: sidebarItem{isTasks: true} has a zero-value (empty
// string) NoteState, which — without explicit isTasks guards — could
// alias real sidebar state machinery keyed by NoteState (e.g. s.expanded,
// a fresh map defaulting every unset key, including "", to false/zero).
// Exercise every keyboard branch (left, right, enter) on the Tasks row and
// confirm the real Inbox section's expand/active state is never touched.
func TestHeadlessSidebarTasksRowDoesNotCorruptInboxState(t *testing.T) {
	m := setupTUI(t)
	m = step(t, m, key("tab"), "tab (-> sidebar)")

	items := m.sidebar.items()
	tasksIdx := -1
	for i, it := range items {
		if it.isTasks {
			tasksIdx = i
			break
		}
	}
	if tasksIdx < 0 {
		t.Fatalf("no Tasks row found: %+v", items)
	}
	m.sidebar.cursor = tasksIdx

	for _, k := range []string{"left", "right", "enter"} {
		msg := key(k)
		if k == "left" || k == "right" {
			msg = tea.KeyMsg{Type: map[string]tea.KeyType{"left": tea.KeyLeft, "right": tea.KeyRight}[k]}
		}
		var cmd tea.Cmd
		m.sidebar, cmd = m.sidebar.update(msg)
		_ = cmd
	}

	if m.sidebar.expanded[vault.StateInbox] {
		t.Error("activating the Tasks row must not expand the Inbox section")
	}
	if m.sidebar.activeState == "" {
		t.Error("sidebar.activeState must not be set to the zero-value NoteState")
	}
}

// TestHeadlessSidebarTasksRowMouseClick covers the mouse-click path into
// the Tasks row, which is a separate code path from keyboard Enter
// (handleMouseClick acts immediately rather than going through
// sidebarModel.update + the m.sidebar.selected dispatcher).
func TestHeadlessSidebarTasksRowMouseClick(t *testing.T) {
	m := setupTUI(t)

	items := m.sidebar.items()
	tasksIdx := -1
	for i, it := range items {
		if it.isTasks {
			tasksIdx = i
			break
		}
	}
	if tasksIdx < 0 {
		t.Fatalf("no Tasks row found: %+v", items)
	}
	// Matches handleMouseClick's itemIdx := y - 4.
	y := tasksIdx + 4
	m = step(t, m, click(2, y), "click Tasks row")

	if m.splits[m.activeSplit].activeView != viewTasksOverview {
		t.Errorf("expected viewTasksOverview after click, got %v", m.splits[m.activeSplit].activeView)
	}
	if !m.sidebar.tasksActive {
		t.Error("expected sidebar.tasksActive=true after click")
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
