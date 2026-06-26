package tui

import (
	"path/filepath"
	"strings"
	"testing"

	"pkm/internal/index"
	"pkm/internal/vault"

	tea "github.com/charmbracelet/bubbletea"
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

// --- substituteLinks ---

func TestSubstituteLinks(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"no links", "no links"},
		{"[[Docker]]", "Docker"},
		{"[[Docker|Container Runtime]]", "Container Runtime"},
		{"see [[A]] and [[B|Alias]] here", "see A and Alias here"},
		{"[[unclosed", "[[unclosed"},
		{"empty [[]] link", "empty  link"},
	}
	for _, tc := range cases {
		got := substituteLinks(tc.in)
		if got != tc.want {
			t.Errorf("substituteLinks(%q) = %q, want %q", tc.in, got, tc.want)
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
	p := newPalette()
	p.input = "  new \"Docker\"  "
	if got := p.value(); got != "new \"Docker\"" {
		t.Errorf("value() = %q", got)
	}
}

// --- palette completion ---

func TestPaletteFiltered(t *testing.T) {
	p := newPalette()

	// Empty input → all commands
	if got := p.filteredSuggestions(); len(got) != len(allCommands) {
		t.Errorf("empty input: want %d suggestions, got %d", len(allCommands), len(got))
	}

	// "n" → only "new"
	p.input = "n"
	sug := p.filteredSuggestions()
	if len(sug) != 1 || sug[0].cmd != "new" {
		t.Errorf("input 'n': got %v", sug)
	}

	// "new " (with space) → locked to "new"
	p.input = "new "
	sug = p.filteredSuggestions()
	if len(sug) != 1 || sug[0].cmd != "new" {
		t.Errorf("input 'new ': got %v", sug)
	}

	// "xyz" → no match
	p.input = "xyz"
	if got := p.filteredSuggestions(); len(got) != 0 {
		t.Errorf("input 'xyz': expected 0 suggestions, got %d", len(got))
	}
}

func TestPaletteTabCompletes(t *testing.T) {
	p := newPalette()
	p.input = "n"
	p, _ = p.update(key("tab"))
	if p.input != "new " {
		t.Errorf("tab completion: got %q, want %q", p.input, "new ")
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

	// Click on sidebar Inbox section header.
	// With pane border, y offsets: breadcrumb(0), top-border(1), SECTIONS(2), blank(3), items(4+)
	// Inbox is items[0] → y=4
	m = step(t, m, click(5, 4), "click sidebar Inbox")
	if m.sidebar.activeState != vault.StateInbox {
		t.Errorf("sidebar state after click: got %v", m.sidebar.activeState)
	}
	if !m.sidebar.expanded[vault.StateInbox] {
		t.Error("Inbox should be expanded after click")
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
	// Two notes were seeded, so items should be: Inbox, note1, note2, Projects, ...
	items := m.sidebar.items()
	if len(items) != len(vault.AllStates)+2 {
		t.Errorf("expected %d items after expand, got %d", len(vault.AllStates)+2, len(items))
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
	if len(items) != len(vault.AllStates) {
		t.Errorf("expected %d items after collapse, got %d", len(vault.AllStates), len(items))
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
