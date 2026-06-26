package tui

import (
	"strconv"
	"strings"

	"pkm/internal/index"
	"pkm/internal/vault"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type view int

const (
	viewList view = iota
	viewNote
)

type pane int

const (
	paneMain pane = iota
	paneSidebar
)

type layout struct {
	sidebarWidth  int // outer width of sidebar box (includes 2 border chars)
	mainWidth     int // total outer width of main pane area
	paneWidth     int // inner content width per split (no border chars)
	contentHeight int // inner content height per pane (no border chars)
}

type Model struct {
	vault  *vault.Vault
	index  *index.Index
	width  int
	height int

	sidebar     sidebarModel
	noteList    noteListModel
	palette     paletteModel
	showPalette bool

	splits      []splitPane
	activeSplit int
	activePane  pane

	statusMsg  string
	panePicker bool
	pickerIdx  int // 0 = sidebar, 1..n = splits[0..n-1]
}

func (m Model) computeLayout() layout {
	sw := m.width / 4
	mw := m.width - sw - 1 // -1 for gap between sidebar and main

	n := len(m.splits)
	if n < 1 {
		n = 1
	}
	// n split panes + (n-1) gaps between them = mw
	paneOuter := (mw - (n - 1)) / n
	if paneOuter < 3 {
		paneOuter = 3
	}
	paneInner := paneOuter - 2
	if paneInner < 1 {
		paneInner = 1
	}

	dropdownH := 0
	if m.showPalette {
		dropdownH = m.palette.dropdownHeight()
	}
	// breadcrumb(1) + top-border(1) + bottom-border(1) + statusbar(1) = 4 reserved
	outerH := m.height - 2 - dropdownH // height of pane boxes incl. borders
	if outerH < 3 {
		outerH = 3
	}
	innerH := outerH - 2 // content rows inside the border
	if innerH < 1 {
		innerH = 1
	}

	return layout{sw, mw, paneInner, innerH}
}

func New(v *vault.Vault, idx *index.Index) Model {
	m := Model{
		vault:      v,
		index:      idx,
		activePane: paneMain,
		splits:     []splitPane{newSplitPane()},
	}
	m.sidebar = newSidebar(idx, v)
	m.noteList = newNoteList(v)
	m.palette = newPalette()

	// Restore session state
	sess := loadSession(v)

	switch sess.Theme {
	case "light":
		activeTheme = LightTheme
	default:
		activeTheme = DarkTheme
	}

	activeState := vault.StateInbox
	if sess.ActiveState != "" {
		activeState = vault.NoteState(sess.ActiveState)
	}
	m.sidebar.activeState = activeState
	for i, s := range vault.AllStates {
		if s == activeState {
			m.sidebar.cursor = i
			break
		}
	}

	// Load notes for the active section so the list is populated on startup.
	if notes, err := v.ListByState(activeState); err == nil {
		m.noteList = m.noteList.withNotes(notes)
	}

	// Restore last open note.
	if sess.LastNoteID != "" {
		if notes, err := v.ListAll(); err == nil {
			for _, n := range notes {
				if n.ID == sess.LastNoteID {
					m.splits[0].openNote(n)
					break
				}
			}
		}
	}

	return m
}

func (m Model) Init() tea.Cmd {
	return m.sidebar.init()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Re-render open notes for the new terminal width.
		l := m.computeLayout()
		for i := range m.splits {
			m.splits[i].viewer = m.splits[i].viewer.preRender(l.paneWidth)
		}

	case tea.MouseMsg:
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			m.handleMouseClick(msg.X, msg.Y)
		}

	case tea.KeyMsg:
		if m.showPalette {
			var cmd tea.Cmd
			m.palette, cmd = m.palette.update(msg)
			cmds = append(cmds, cmd)
			if m.palette.submitted {
				result, cmd := m.handleCommand(m.palette.value())
				if result != "" {
					m.statusMsg = result
				}
				m.showPalette = false
				cmds = append(cmds, cmd)
			} else if m.palette.cancelled {
				m.showPalette = false
			}
			return m, tea.Batch(cmds...)
		}

		m.statusMsg = ""

		// Pane picker mode captures all input.
		if m.panePicker {
			switch msg.String() {
			case "left", "h":
				if m.pickerIdx > 0 {
					m.pickerIdx--
				}
			case "right", "l":
				if m.pickerIdx < len(m.splits) {
					m.pickerIdx++
				}
			case "enter", "ctrl+p":
				m.applyPickerSelection()
				m.panePicker = false
			case "esc":
				m.panePicker = false
			}
			return m, nil
		}

		switch msg.String() {
		case "ctrl+c", "q":
			saveSession(m.vault, &m)
			return m, tea.Quit

		case ":":
			m.showPalette = true
			m.palette = newPalette()
			return m, nil

		case "ctrl+p":
			m.panePicker = true
			if m.activePane == paneSidebar {
				m.pickerIdx = 0
			} else {
				m.pickerIdx = m.activeSplit + 1
			}
			return m, nil

		case "ctrl+s":
			m.showPalette = true
			m.palette = newPaletteWithInput("search ")
			return m, nil

		case "e":
			sp := &m.splits[m.activeSplit]
			if sp.activeView == viewNote && sp.viewer.note != nil {
				return m, openInEditor(m.vault, sp.viewer.note)
			}

		case "tab":
			if m.activePane == paneMain {
				m.activePane = paneSidebar
			} else {
				m.activePane = paneMain
			}

		case "ctrl+w":
			if len(m.splits) > 1 {
				m.activeSplit = (m.activeSplit + 1) % len(m.splits)
				m.activePane = paneMain
			}

		case "[", "alt+left":
			m.splits[m.activeSplit].back()

		case "]", "alt+right":
			m.splits[m.activeSplit].forward()
		}

		if m.activePane == paneSidebar {
			var cmd tea.Cmd
			m.sidebar, cmd = m.sidebar.update(msg)
			cmds = append(cmds, cmd)
			if m.sidebar.selected {
				m.sidebar.selected = false
				if m.sidebar.selectedNote != nil {
					// Note title chosen: open it in the main pane and switch focus.
					n := m.sidebar.selectedNote
					m.sidebar.selectedNote = nil
					m.splits[m.activeSplit].openNote(n)
					l := m.computeLayout()
					m.splits[m.activeSplit].viewer = m.splits[m.activeSplit].viewer.preRender(l.paneWidth)
					m.activePane = paneMain
				} else {
					// Section expanded/collapsed: reload note list, stay in sidebar.
					if notes, err := m.vault.ListByState(m.sidebar.activeState); err == nil {
						m.noteList = m.noteList.withNotes(notes)
					}
				}
			}
		} else {
			sp := &m.splits[m.activeSplit]
			switch sp.activeView {
			case viewList:
				var cmd tea.Cmd
				m.noteList, cmd = m.noteList.update(msg)
				cmds = append(cmds, cmd)
				if m.noteList.chosen != nil {
					sp.openNote(m.noteList.chosen)
					l := m.computeLayout()
					sp.viewer = sp.viewer.preRender(l.paneWidth)
					m.noteList.chosen = nil
				}
			case viewNote:
				var cmd tea.Cmd
				sp.viewer, cmd = sp.viewer.update(msg)
				cmds = append(cmds, cmd)
				if sp.viewer.back {
					sp.viewer.back = false
					if !sp.back() {
						sp.activeView = viewList
					}
				}
			}
		}

	case vaultChangedMsg:
		counts, _ := m.index.CountByState()
		m.sidebar = m.sidebar.withCounts(counts)
		m.sidebar = m.sidebar.refreshNotes()
		if msg.note != nil {
			l := m.computeLayout()
			for i := range m.splits {
				if m.splits[i].viewer.note != nil && m.splits[i].viewer.note.ID == msg.note.ID {
					m.splits[i].viewer = m.splits[i].viewer.withNote(msg.note)
					m.splits[i].viewer = m.splits[i].viewer.preRender(l.paneWidth)
				}
			}
		}

	case editorFinishedMsg:
		if msg.err != nil {
			m.statusMsg = "editor error: " + msg.err.Error()
		} else if msg.note != nil {
			m.index.Upsert(msg.note)
			sp := &m.splits[m.activeSplit]
			sp.viewer = sp.viewer.withNote(msg.note)
			l := m.computeLayout()
			sp.viewer = sp.viewer.preRender(l.paneWidth)
			m.statusMsg = "saved: " + msg.note.Title
		}

	case statusMsg:
		m.statusMsg = string(msg)

	case notesLoadedMsg:
		m.noteList = m.noteList.withNotes(msg.notes)
		m.splits[m.activeSplit].activeView = viewList

	case countsRefreshedMsg:
		m.sidebar = m.sidebar.withCounts(msg.counts)
	}

	return m, tea.Batch(cmds...)
}

func (m *Model) applyPickerSelection() {
	if m.pickerIdx == 0 {
		m.activePane = paneSidebar
	} else {
		m.activeSplit = m.pickerIdx - 1
		if m.activeSplit >= len(m.splits) {
			m.activeSplit = len(m.splits) - 1
		}
		m.activePane = paneMain
	}
}

func (m *Model) handleMouseClick(x, y int) {
	l := m.computeLayout()
	m.panePicker = false // any click exits picker mode

	// Click in sidebar area (x < sidebarWidth = outer width including border)
	if x < l.sidebarWidth {
		// y offsets with border: breadcrumb(0), top-border(1), SECTIONS(2), blank(3), items(4+)
		itemIdx := y - 4
		items := m.sidebar.items()
		if itemIdx < 0 || itemIdx >= len(items) {
			return
		}
		m.sidebar.cursor = itemIdx
		item := items[itemIdx]
		m.activePane = paneSidebar

		if item.isSection {
			// Toggle expand/collapse.
			if m.sidebar.expanded[item.state] {
				m.sidebar.expanded[item.state] = false
				// Clamp cursor if it was on a note that just disappeared.
				newItems := m.sidebar.items()
				if m.sidebar.cursor >= len(newItems) {
					m.sidebar.cursor = len(newItems) - 1
				}
			} else {
				m.sidebar.expanded[item.state] = true
				notes, _ := m.vault.ListByState(item.state)
				m.sidebar.notesByState[item.state] = notes
			}
			m.sidebar.activeState = item.state
			if notes, err := m.vault.ListByState(item.state); err == nil {
				m.noteList = m.noteList.withNotes(notes)
			}
		} else {
			// Click on note title: open directly.
			m.splits[m.activeSplit].openNote(item.note)
			l := m.computeLayout()
			m.splits[m.activeSplit].viewer = m.splits[m.activeSplit].viewer.preRender(l.paneWidth)
			m.activePane = paneMain
		}
		return
	}

	// Click in main pane area (past sidebar + 1-char gap)
	if x >= l.sidebarWidth+1 && l.paneWidth > 0 {
		mainX := x - l.sidebarWidth - 1
		paneOuter := l.paneWidth + 2        // inner + left+right border
		slotWidth := paneOuter + 1          // pane outer + gap between panes
		clickedSplit := mainX / slotWidth
		if clickedSplit >= len(m.splits) {
			clickedSplit = len(m.splits) - 1
		}
		m.activeSplit = clickedSplit
		m.activePane = paneMain

		sp := &m.splits[m.activeSplit]
		if sp.activeView == viewList {
			// y offsets with border: breadcrumb(0), top-border(1), noteList-padding(2), notes(3+)
			noteIdx := y - 3
			if noteIdx >= 0 && noteIdx < len(m.noteList.notes) {
				m.noteList.cursor = noteIdx
				sp.openNote(m.noteList.notes[noteIdx])
				l := m.computeLayout()
				sp.viewer = sp.viewer.preRender(l.paneWidth)
			}
		}
	}
}

func (m Model) View() string {
	if m.width == 0 {
		return ""
	}

	l := m.computeLayout()

	sidebarFocused := m.activePane == paneSidebar && !m.panePicker
	sbInner := l.sidebarWidth - 2
	if sbInner < 1 {
		sbInner = 1
	}

	sbContent := m.sidebar.render(sbInner, l.contentHeight, sidebarFocused)

	sbBorderColor := activeTheme.BorderNormal
	if m.panePicker && m.pickerIdx == 0 {
		sbBorderColor = activeTheme.BorderPicker
	} else if sidebarFocused {
		sbBorderColor = activeTheme.BorderFocus
	}

	sbBox := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(sbBorderColor).
		Width(sbInner).
		Height(l.contentHeight).
		Render(sbContent)

	main := m.renderSplits(l)

	// 1-char gap between sidebar and main; height must match bordered pane height
	outerH := l.contentHeight + 2
	gap := lipgloss.NewStyle().Width(1).Height(outerH).Render("")

	body := lipgloss.JoinHorizontal(lipgloss.Top, sbBox, gap, main)

	parts := []string{m.renderBreadcrumb(), body}

	if m.showPalette {
		dh := m.palette.dropdownHeight()
		if dh > 0 {
			parts = append(parts, m.palette.renderDropdown(m.width))
		}
		parts = append(parts, m.palette.renderInputLine(m.width))
	} else {
		parts = append(parts, m.renderTooltipBar())
	}

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m Model) renderSplits(l layout) string {
	n := len(m.splits)
	if n == 0 || l.paneWidth <= 0 {
		return ""
	}

	// Distribute remainder width to the last pane.
	mw := l.mainWidth
	paneOuter := (mw - (n - 1)) / n
	if paneOuter < 3 {
		paneOuter = 3
	}
	remainder := mw - (n-1) - paneOuter*n

	outerH := l.contentHeight + 2 // height of bordered boxes (for gap sizing)

	var parts []string
	for i := range m.splits {
		po := paneOuter
		if i == n-1 {
			po += remainder
		}
		pi := po - 2
		if pi < 1 {
			pi = 1
		}

		focused := m.activePane == paneMain && i == m.activeSplit && !m.panePicker
		sp := m.splits[i]

		var content string
		switch sp.activeView {
		case viewList:
			content = m.noteList.render(pi, l.contentHeight, focused)
		case viewNote:
			content = sp.viewer.render(pi, l.contentHeight, focused)
		}

		borderColor := activeTheme.BorderNormal
		if m.panePicker && m.pickerIdx == i+1 {
			borderColor = activeTheme.BorderPicker
		} else if focused {
			borderColor = activeTheme.BorderFocus
		}

		box := lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(borderColor).
			Width(pi).
			Height(l.contentHeight).
			Render(content)

		if i > 0 {
			gap := lipgloss.NewStyle().Width(1).Height(outerH).Render("")
			parts = append(parts, gap)
		}
		parts = append(parts, box)
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

func (m Model) renderBreadcrumb() string {
	sp := m.splits[m.activeSplit]

	title := " PKM"
	if sp.activeView == viewNote && sp.viewer.note != nil {
		title += "  ›  " + sp.viewer.note.Title
	} else if m.sidebar.activeState != "" {
		title += "  ›  " + capitalize(string(m.sidebar.activeState))
	}

	paneTag := ""
	if len(m.splits) > 1 {
		paneTag = " [" + strconv.Itoa(m.activeSplit+1) + "/" + strconv.Itoa(len(m.splits)) + "]"
	}

	return lipgloss.NewStyle().
		Width(m.width).
		Background(activeTheme.Accent).
		Foreground(activeTheme.AccentFg).
		Bold(true).
		Render(title + paneTag)
}

func (m Model) renderTooltipBar() string {
	chip := func(key, action string) string {
		k := lipgloss.NewStyle().
			Background(activeTheme.Accent).
			Foreground(activeTheme.AccentFg).
			Bold(true).
			Padding(0, 1).
			Render(key)
		a := lipgloss.NewStyle().
			Background(activeTheme.BlurredBg).
			Foreground(activeTheme.TextPrimary).
			Padding(0, 1).
			Render(action)
		return k + a
	}

	var chips []string

	if m.panePicker {
		total := len(m.splits) + 1 // sidebar + splits
		chips = append(chips,
			chip("←/→", "select pane"),
			chip("↵", "confirm"),
			chip("Esc", "cancel"),
		)
		bar := strings.Join(chips, " ")
		bar += lipgloss.NewStyle().
			Foreground(activeTheme.Cursor).
			Render(strconv.Itoa(m.pickerIdx+1)+"/"+strconv.Itoa(total))
		return lipgloss.NewStyle().
			Width(m.width).
			Background(activeTheme.StatusBg).
			Render(bar)
	}

	chips = append(chips, chip(":", "command"), chip("q", "quit"), chip("^P", "panes"), chip("^S", "search"))

	if m.activePane == paneSidebar {
		chips = append(chips,
			chip("↑↓", "navigate"),
			chip("←→", "collapse/expand"),
			chip("↵", "select"),
			chip("Tab", "→ panes"),
		)
	} else {
		chips = append(chips, chip("Tab", "sidebar"))
		sp := m.splits[m.activeSplit]
		if sp.activeView == viewList {
			chips = append(chips,
				chip("↑↓ j k", "navigate"),
				chip("↵", "open note"),
			)
		} else {
			chips = append(chips,
				chip("↑↓ j k", "scroll"),
				chip("e", "edit"),
				chip("[/]", "history"),
				chip("Esc", "back"),
			)
		}
		if len(m.splits) > 1 {
			chips = append(chips, chip("^W", "next pane"))
		}
	}

	bar := strings.Join(chips, " ")
	if m.statusMsg != "" {
		bar += lipgloss.NewStyle().
			Foreground(activeTheme.Cursor).
			Render("  " + m.statusMsg)
	}

	return lipgloss.NewStyle().
		Width(m.width).
		Background(activeTheme.StatusBg).
		Render(bar)
}

func capitalize(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

type statusMsg string
type notesLoadedMsg struct{ notes []*vault.Note }
type countsRefreshedMsg struct{ counts map[vault.NoteState]int }
