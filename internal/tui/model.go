package tui

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"pkm/internal/index"
	"pkm/internal/vault"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func timeNow() time.Time { return time.Now().Truncate(time.Second) }

type view int

const (
	viewList view = iota
	viewNote
	viewEdit
	viewProjectsOverview
	viewProjectDetail
	viewHelp
	viewSectionLanding
)

type undoRecord struct {
	oldNote vault.Note
	newNote vault.Note
}

type tickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(30*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

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

	pendingMoveNote *vault.Note // note awaiting project assignment before move to projects

	undoStack []undoRecord
	redoStack []undoRecord

	cfg        AppConfig
	showConfig bool
	configView configPane

	titleSet map[string]bool // lowercase note titles → used for link existence checks
}

func (m Model) computeLayout() layout {
	swPct := m.cfg.SidebarWidth
	if swPct <= 0 {
		swPct = 25
	}
	sw := m.width * swPct / 100
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

// resizeOpenEditors re-renders each split's viewer and, for any split
// currently in the editor, resizes the textarea/inputs to fit the given
// layout. Called whenever available space changes: terminal resize, config
// close, or the palette opening/closing over an active edit session.
func (m *Model) resizeOpenEditors(l layout) {
	for i := range m.splits {
		m.splits[i].viewer = m.splits[i].viewer.preRender(l.paneWidth, m.titleSet)
		if m.splits[i].activeView == viewEdit {
			inputW := max(1, l.paneWidth-editLabelWidth)
			m.splits[i].editor.tagsInput.Width = inputW
			m.splits[i].editor.projInput.Width = inputW
			m.splits[i].editor.ta.SetWidth(l.paneWidth)
			m.splits[i].editor.contentHeight = l.contentHeight
			m.splits[i].editor.ta.SetHeight(m.splits[i].editor.bodyHeight())
		}
	}
}

// commitEditorDraft saves a split's in-progress edit and returns it to the
// note viewer. Used by Ctrl+S, and by the palette when a command other than
// :insert is run mid-edit — those commands act on the saved note, so the
// draft must be committed first or a later save would clobber them.
func (m *Model) commitEditorDraft(sp *splitPane) {
	n := sp.editor.note
	oldNote := *n // snapshot before mutation
	n.Body = sp.editor.ta.Value()
	n.State = vault.AllStates[sp.editor.stateIdx]
	n.Tags = parseTags(sp.editor.tagsInput.Value())
	n.Project = strings.TrimSpace(sp.editor.projInput.Value())
	if saveErr := m.vault.Save(n); saveErr != nil {
		m.statusMsg = "save error: " + saveErr.Error()
	} else {
		rec := undoRecord{oldNote: oldNote, newNote: *n}
		m.undoStack = append(m.undoStack, rec)
		if len(m.undoStack) > 20 {
			m.undoStack = m.undoStack[1:]
		}
		m.redoStack = nil
		m.index.Upsert(n)
		sp.viewer = sp.viewer.withNote(n)
		l := m.computeLayout()
		sp.viewer = sp.viewer.preRender(l.paneWidth, m.titleSet)
		m.statusMsg = "saved: " + n.Title
	}
	sp.editor = editPane{}
	sp.activeView = viewNote
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
	m.palette = newPalette(nil, nil)
	m.titleSet = buildTitleSet(v)

	// Load config first (theme, sidebar width, etc.).
	cfg := loadConfig(v)
	m.cfg = cfg

	activeTheme = NordTheme // default if theme name not recognized
	for _, t := range ThemeChoices {
		if t.Name == cfg.Theme {
			activeTheme = t
			break
		}
	}

	// Restore session state.
	sess := loadSession(v)

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

	// Restore last open note (skip if restore_session is off).
	if cfg.RestoreSession && sess.LastNoteID != "" {
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
	return tea.Batch(m.sidebar.init(), tickCmd())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Re-render open notes and resize active edit panes for the new terminal width.
		m.resizeOpenEditors(m.computeLayout())

	case tea.MouseMsg:
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			m.handleMouseClick(msg.X, msg.Y)
		}

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		}
		if m.showPalette {
			var cmd tea.Cmd
			m.palette, cmd = m.palette.update(msg)
			cmds = append(cmds, cmd)
			if m.palette.submitted {
				raw := m.palette.value()
				// A command other than :insert acts on the saved note, not the
				// live buffer — commit the draft first so the two can't diverge.
				// :insert writes straight into the open editor (see cmdInsert).
				if len(m.splits) > 0 {
					if sp := &m.splits[m.activeSplit]; sp.activeView == viewEdit && !isLiveEditCommand(raw) {
						m.commitEditorDraft(sp)
					}
				}
				result, cmd := m.handleCommand(raw)
				if result != "" {
					m.statusMsg = result
				}
				m.showPalette = false
				m.resizeOpenEditors(m.computeLayout())
				cmds = append(cmds, cmd)
			} else if m.palette.cancelled {
				m.showPalette = false
				m.resizeOpenEditors(m.computeLayout())
			}
			return m, tea.Batch(cmds...)
		}

		// Edit mode captures all input — bypass global shortcuts.
		if m.activePane == paneMain && len(m.splits) > 0 {
			sp := &m.splits[m.activeSplit]
			if sp.activeView == viewEdit {
				// Ctrl+Space opens the palette as an overlay on top of the still-
				// live editor (draft untouched) — the only way to reach it while
				// editing, since a bare ":" must stay a literal character in note
				// bodies. See the showPalette branch above for what runs on submit.
				if msg.String() == m.cfg.Keymap.Palette {
					m.showPalette = true
					m.palette = newPalette(sortedNotes(m.vault), m.vault.Projects.ActiveNames()).withVariables(m.variableNames()).withContext(ctxEditing)
					m.resizeOpenEditors(m.computeLayout())
					return m, nil
				}
				var cmd tea.Cmd
				sp.editor, cmd = sp.editor.update(msg)
				cmds = append(cmds, cmd)
				if sp.editor.saved {
					m.commitEditorDraft(sp)
				} else if sp.editor.cancelled {
					sp.editor = editPane{}
					sp.activeView = viewNote
				}
				return m, tea.Batch(cmds...)
			}

			// Project Detail's bridge-entry field captures all input — bypass
			// global shortcuts the same way Editor/Palette/Config/PanePicker do.
			if sp.activeView == viewProjectDetail && sp.projectDetail.editingBridge {
				m.updateProjectDetail(sp, msg)
				return m, nil
			}
		}

		// Config overlay captures all input.
		if m.showConfig {
			switch msg.String() {
			case "esc":
				if nv, consumed := m.configView.cancelInPlace(); consumed {
					m.configView = nv
					return m, nil
				}
				m.showConfig = false
				m.cfg.Keymap = sliceToKeymap(m.configView.kbKeys)
				m.cfg.Variables = m.configView.variablesMap()
				saveConfig(m.vault, m.cfg)
				// Treat config-close like a resize: rerender all viewers at
				// potentially new layout (sidebar width) and theme.
				m.resizeOpenEditors(m.computeLayout())
				return m, nil
			case "tab":
				if !m.configView.busy() {
					m.configView = m.configView.nextSection()
				}
				return m, nil
			case "shift+tab":
				if !m.configView.busy() {
					m.configView = m.configView.prevSection()
				}
				return m, nil
			}

			switch m.configView.section {
			case secKeybindings:
				m.configView = m.configView.updateKeybindings(msg)
			case secVariables:
				m.configView = m.configView.updateVariables(msg)
			default:
				switch msg.String() {
				case "j", "down":
					m.configView = m.configView.moveCursor(1)
				case "k", "up":
					m.configView = m.configView.moveCursor(-1)
				case "left", "h":
					m.configView = m.configView.changeValue(-1)
					m.applyConfigItem(m.configView.cursor, m.configView.values[m.configView.cursor])
				case "right", "l", "enter":
					m.configView = m.configView.changeValue(1)
					m.applyConfigItem(m.configView.cursor, m.configView.values[m.configView.cursor])
				}
			}
			return m, nil
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
			case "enter", m.cfg.Keymap.PanePicker:
				m.applyPickerSelection()
				m.panePicker = false
			case "esc":
				m.panePicker = false
			}
			return m, nil
		}

		switch msg.String() {
		case m.cfg.Keymap.Quit, "ctrl+d":
			saveSession(m.vault, &m)
			return m, tea.Quit

		case m.cfg.Keymap.Undo:
			m.handleUndo()
			return m, nil

		case m.cfg.Keymap.Redo:
			m.handleRedo()
			return m, nil

		case ":", m.cfg.Keymap.Palette:
			m.showPalette = true
			ctx := ctxDefault
			if len(m.splits) > 0 && m.splits[m.activeSplit].activeView == viewNote {
				ctx = ctxNoteOpen
			}
			m.palette = newPalette(sortedNotes(m.vault), m.vault.Projects.ActiveNames()).withVariables(m.variableNames()).withContext(ctx)
			return m, nil

		case m.cfg.Keymap.PanePicker:
			m.panePicker = true
			if m.activePane == paneSidebar {
				m.pickerIdx = 0
			} else {
				m.pickerIdx = m.activeSplit + 1
			}
			return m, nil

		case "?":
			sp := &m.splits[m.activeSplit]
			if sp.activeView == viewHelp {
				sp.activeView = viewList
			} else {
				sp.activeView = viewHelp
				sp.helpScrollOff = 0
			}
			m.activePane = paneMain
			return m, nil

		case "N":
			m.showPalette = true
			m.palette = newPaletteWithInput("new ", sortedNotes(m.vault), m.vault.Projects.ActiveNames()).withVariables(m.variableNames())
			return m, nil

		case "O", "S":
			m.showPalette = true
			m.palette = newPaletteWithInput("open ", sortedNotes(m.vault), m.vault.Projects.ActiveNames()).withVariables(m.variableNames())
			return m, nil

		case "A":
			m.showPalette = true
			input := "archive "
			if n := m.splits[m.activeSplit].viewer.note; m.splits[m.activeSplit].activeView == viewNote && n != nil {
				input += n.Title
			}
			m.palette = newPaletteWithInput(input, sortedNotes(m.vault), m.vault.Projects.ActiveNames()).withVariables(m.variableNames())
			return m, nil

		case "M":
			m.showPalette = true
			input := "move "
			if n := m.splits[m.activeSplit].viewer.note; m.splits[m.activeSplit].activeView == viewNote && n != nil {
				input += n.Title + " → "
			}
			m.palette = newPaletteWithInput(input, sortedNotes(m.vault), m.vault.Projects.ActiveNames()).withVariables(m.variableNames())
			return m, nil

		case "T":
			m.showPalette = true
			m.palette = newPaletteWithInput("insert ", sortedNotes(m.vault), m.vault.Projects.ActiveNames()).withVariables(m.variableNames())
			return m, nil

		case "P":
			m.showPalette = true
			m.palette = newPaletteWithInput("add project ", sortedNotes(m.vault), m.vault.Projects.ActiveNames()).withVariables(m.variableNames())
			return m, nil

		case "e":
			sp := &m.splits[m.activeSplit]
			if sp.activeView == viewNote && sp.viewer.note != nil {
				l := m.computeLayout()
				var cmd tea.Cmd
				sp.editor, cmd = newEditPane(sp.viewer.note, l.paneWidth, l.contentHeight, sortedNotes(m.vault), m.cfg.LineNumbers, m.cfg.Keymap.Save)
				sp.activeView = viewEdit
				m.activePane = paneMain
				return m, cmd
			}

		case "tab":
			if m.activePane == paneMain {
				m.activePane = paneSidebar
			} else {
				m.activePane = paneMain
			}

		case m.cfg.Keymap.NextPane:
			if len(m.splits) > 1 {
				m.activeSplit = (m.activeSplit + 1) % len(m.splits)
				m.activePane = paneMain
			}

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
					m.splits[m.activeSplit].viewer = m.splits[m.activeSplit].viewer.preRender(l.paneWidth, m.titleSet)
					m.activePane = paneMain
				} else if m.sidebar.selectedProject != nil {
					// Project entry chosen: open project detail view.
					p := m.sidebar.selectedProject
					m.sidebar.selectedProject = nil
					notes, _ := m.vault.ListAll()
					var pNotes []*vault.Note
					for _, n := range notes {
						if n.Project == p.Name && n.State == vault.StateProjects {
							pNotes = append(pNotes, n)
						}
					}
					m.splits[m.activeSplit].projectDetail = newProjectDetailPane(p, pNotes)
					m.splits[m.activeSplit].activeView = viewProjectDetail
					m.activePane = paneMain
				} else {
					// Section header selected: show landing page; keep focus in sidebar
					// so the user can navigate notes below and open them with Enter.
					m.showSectionLanding(m.sidebar.activeState, m.sidebar.templatesActive)
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
					sp.viewer = sp.viewer.preRender(l.paneWidth, m.titleSet)
					m.noteList.chosen = nil
				}
			case viewNote:
				switch msg.String() {
				case "[", "alt+left":
					if sp.back() {
						l := m.computeLayout()
						sp.viewer = sp.viewer.preRender(l.paneWidth, m.titleSet)
					}
				case "]", "alt+right":
					if sp.forward() {
						l := m.computeLayout()
						sp.viewer = sp.viewer.preRender(l.paneWidth, m.titleSet)
					}
				default:
					var cmd tea.Cmd
					sp.viewer, cmd = sp.viewer.update(msg)
					cmds = append(cmds, cmd)
					if sp.viewer.back {
						sp.viewer.back = false
						if !sp.back() {
							sp.activeView = viewList
						} else {
							l := m.computeLayout()
							sp.viewer = sp.viewer.preRender(l.paneWidth, m.titleSet)
						}
					}
				}
			case viewProjectDetail:
				m.updateProjectDetail(sp, msg)
			case viewProjectsOverview:
				if msg.String() == "esc" || msg.String() == "backspace" {
					sp.activeView = viewList
				}
			case viewSectionLanding:
				if msg.String() == "esc" || msg.String() == "backspace" {
					sp.activeView = viewList
				}
			case viewHelp:
				switch msg.String() {
				case "esc", "backspace", "?":
					sp.activeView = viewList
				case "j", "down":
					l := m.computeLayout()
					maxOff := HelpTotalLines() - l.contentHeight
					if maxOff < 0 {
						maxOff = 0
					}
					if sp.helpScrollOff < maxOff {
						sp.helpScrollOff++
					}
				case "k", "up":
					if sp.helpScrollOff > 0 {
						sp.helpScrollOff--
					}
				case "g":
					sp.helpScrollOff = 0
				case "G":
					l := m.computeLayout()
					maxOff := HelpTotalLines() - l.contentHeight
					if maxOff < 0 {
						maxOff = 0
					}
					sp.helpScrollOff = maxOff
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
					m.splits[i].viewer = m.splits[i].viewer.preRender(l.paneWidth, m.titleSet)
				}
			}
		}

	case statusMsg:
		m.statusMsg = string(msg)

	case notesLoadedMsg:
		m.noteList = m.noteList.withNotes(msg.notes)
		m.splits[m.activeSplit].activeView = viewList

	case countsRefreshedMsg:
		m.sidebar = m.sidebar.withCounts(msg.counts)
		m.sidebar.templateCount = msg.templateCount

	case tickMsg:
		// Clock tick: re-render so the breadcrumb time stays current.
		return m, tickCmd()

	default:
		// Route unrecognised messages (e.g. cursor-blink ticks) to the active edit pane.
		if m.activePane == paneMain && len(m.splits) > 0 {
			sp := &m.splits[m.activeSplit]
			if sp.activeView == viewEdit {
				var cmd tea.Cmd
				sp.editor, cmd = sp.editor.update(msg)
				cmds = append(cmds, cmd)
			}
		}
	}

	return m, tea.Batch(cmds...)
}

func (m *Model) applyConfigItem(itemIdx, valueIdx int) {
	switch itemIdx {
	case cfgItemTheme:
		if valueIdx >= 0 && valueIdx < len(ThemeChoices) {
			activeTheme = ThemeChoices[valueIdx]
			m.cfg.Theme = activeTheme.Name
			m.bustViewerCaches()
		}
	case cfgItemSidebarWidth:
		widths := []int{20, 25, 33}
		if valueIdx >= 0 && valueIdx < len(widths) {
			m.cfg.SidebarWidth = widths[valueIdx]
		}
		m.bustViewerCaches() // cached render is now the wrong width
	case cfgItemRestoreSession:
		m.cfg.RestoreSession = valueIdx == 0
	case cfgItemLineNumbers:
		m.cfg.LineNumbers = valueIdx == 0
	}
}

// applyConfig replaces the running config wholesale (used by :config import)
// and re-applies every side effect a piecemeal change would normally trigger:
// active theme, cached renders, and the config overlay if it's open.
func (m *Model) applyConfig(cfg AppConfig) {
	m.cfg = cfg
	activeTheme = NordTheme
	for _, t := range ThemeChoices {
		if t.Name == cfg.Theme {
			activeTheme = t
			break
		}
	}
	m.bustViewerCaches()
	if m.showConfig {
		m.configView = newConfigPane(m.cfg)
	}
	m.resizeOpenEditors(m.computeLayout())
}

func (m *Model) bustViewerCaches() {
	for i := range m.splits {
		m.splits[i].viewer.rendered = ""
		m.splits[i].viewer.renderWidth = 0
	}
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
			if item.isTemplates {
				if m.sidebar.templatesExpanded {
					m.sidebar.templatesExpanded = false
					newItems := m.sidebar.items()
					if m.sidebar.cursor >= len(newItems) {
						m.sidebar.cursor = len(newItems) - 1
					}
				} else {
					m.sidebar.templatesExpanded = true
					notes, _ := m.vault.ListByTag("template")
					m.sidebar.templateNotes = notes
					m.sidebar.templateCount = len(notes)
				}
				m.sidebar.templatesActive = true
			} else {
				if m.sidebar.expanded[item.state] {
					m.sidebar.expanded[item.state] = false
					newItems := m.sidebar.items()
					if m.sidebar.cursor >= len(newItems) {
						m.sidebar.cursor = len(newItems) - 1
					}
				} else {
					m.sidebar.expanded[item.state] = true
					if item.state != vault.StateProjects {
						notes, _ := m.vault.ListByState(item.state)
						m.sidebar.notesByState[item.state] = notes
					}
				}
				m.sidebar.activeState = item.state
				m.sidebar.templatesActive = false
				m.sidebar.projectsActive = item.state == vault.StateProjects
			}
			m.showSectionLanding(m.sidebar.activeState, m.sidebar.templatesActive)
		} else if item.isProjectEntry {
			if m.sidebar.expandedProjects[item.project.Name] {
				m.sidebar.expandedProjects[item.project.Name] = false
				newItems := m.sidebar.items()
				if m.sidebar.cursor >= len(newItems) {
					m.sidebar.cursor = len(newItems) - 1
				}
			} else {
				m.sidebar.expandedProjects[item.project.Name] = true
				allNotes, _ := m.vault.ListAll()
				var pNotes []*vault.Note
				for _, n := range allNotes {
					if n.Project == item.project.Name && n.State == vault.StateProjects {
						pNotes = append(pNotes, n)
					}
				}
				m.sidebar.projectNotesByName[item.project.Name] = pNotes
			}
			m.sidebar.activeProjectName = item.project.Name
			m.sidebar.projectsActive = true
			m.sidebar.templatesActive = false
			allNotes, _ := m.vault.ListAll()
			var pNotes []*vault.Note
			for _, n := range allNotes {
				if n.Project == item.project.Name && n.State == vault.StateProjects {
					pNotes = append(pNotes, n)
				}
			}
			m.splits[m.activeSplit].projectDetail = newProjectDetailPane(item.project, pNotes)
			m.splits[m.activeSplit].activeView = viewProjectDetail
			m.activePane = paneMain
		} else {
			// Click on note (regular or project note): open directly.
			if item.note != nil {
				m.splits[m.activeSplit].openNote(item.note)
				l := m.computeLayout()
				m.splits[m.activeSplit].viewer = m.splits[m.activeSplit].viewer.preRender(l.paneWidth, m.titleSet)
				m.activePane = paneMain
			}
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
				sp.viewer = sp.viewer.preRender(l.paneWidth, m.titleSet)
			}
		} else if sp.activeView == viewNote {
			// y offsets: breadcrumb(0), top-border(1), content starts at y=2.
			// The sticky header occupies the first headerLineCount rows of content,
			// so subtract those to get a body-relative line index.
			contentRow := y - 2
			// +1 for the fold separator line drawn below the header.
			if contentRow <= sp.viewer.headerLineCount {
				return // click is in the sticky header or fold separator, no links there
			}
			bodyLine := (contentRow - sp.viewer.headerLineCount - 1) + sp.viewer.scrollOff
			if target := sp.viewer.linkAtLine(bodyLine); target != "" {
				m.openOrCreateNote(target)
			}
		}
	}
}

// openOrCreateNote navigates to the named note, creating it if it doesn't exist.
func (m *Model) openOrCreateNote(title string) {
	n, err := m.vault.FindByTitle(title)
	if err != nil {
		n, err = m.vault.Create(title)
		if err != nil {
			m.statusMsg = "error: " + err.Error()
			return
		}
		m.index.Upsert(n)
		m.titleSet[strings.ToLower(n.Title)] = true
	}
	sp := &m.splits[m.activeSplit]
	sp.openNote(n)
	l := m.computeLayout()
	sp.viewer = sp.viewer.preRender(l.paneWidth, m.titleSet)
	m.activePane = paneMain
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

	activeNoteID := ""
	if len(m.splits) > 0 && m.splits[m.activeSplit].viewer.note != nil {
		activeNoteID = m.splits[m.activeSplit].viewer.note.ID
	}
	sbContent := m.sidebar.render(sbInner, l.contentHeight, sidebarFocused, activeNoteID)

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

	var main string
	if m.showConfig {
		configInner := l.mainWidth - 2
		if configInner < 1 {
			configInner = 1
		}
		main = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(activeTheme.BorderFocus).
			Width(configInner).
			Height(l.contentHeight).
			Render(m.configView.render(configInner, l.contentHeight))
	} else {
		main = m.renderSplits(l)
	}

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
		case viewEdit:
			content = sp.editor.render(pi, l.contentHeight)
		case viewProjectsOverview:
			content = renderProjectsOverview(sp.allProjects, sp.notes, pi, l.contentHeight)
		case viewProjectDetail:
			content = sp.projectDetail.render(pi, l.contentHeight)
		case viewHelp:
			content = renderHelpView(pi, l.contentHeight, sp.helpScrollOff)
		case viewSectionLanding:
			content = sp.sectionLanding.render(pi, l.contentHeight)
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
	switch sp.activeView {
	case viewNote:
		if sp.viewer.note != nil {
			title += "  ›  " + sp.viewer.note.Title
		}
	case viewProjectDetail:
		if sp.projectDetail.project != nil {
			title += "  ›  Projects  ›  " + sp.projectDetail.project.Name
		}
	case viewProjectsOverview:
		title += "  ›  Projects"
	case viewHelp:
		title += "  ›  Help"
	case viewSectionLanding:
		if sp.sectionLanding.isTemplates {
			title += "  ›  #templates"
		} else {
			title += "  ›  " + capitalize(string(sp.sectionLanding.state))
		}
	default:
		if m.sidebar.activeState != "" {
			title += "  ›  " + capitalize(string(m.sidebar.activeState))
		}
	}

	paneTag := ""
	if len(m.splits) > 1 {
		paneTag = " [" + strconv.Itoa(m.activeSplit+1) + "/" + strconv.Itoa(len(m.splits)) + "]"
	}

	now := time.Now()
	dateStr := now.Format("Mon 02/01/2006 15:04") + " "

	left := title + paneTag
	pad := m.width - len([]rune(left)) - len([]rune(dateStr))
	if pad < 1 {
		pad = 1
	}
	content := left + strings.Repeat(" ", pad) + dateStr

	return lipgloss.NewStyle().
		Width(m.width).
		Background(activeTheme.Accent).
		Foreground(activeTheme.AccentFg).
		Bold(true).
		Render(content)
}

// keyChipLabel renders a bubbletea key string (e.g. "ctrl+p") as a short
// display label (e.g. "^P") for the tooltip bar, reflecting whatever the
// user has remapped it to.
func keyChipLabel(key string) string {
	switch {
	case strings.HasPrefix(key, "ctrl+"):
		rest := strings.TrimPrefix(key, "ctrl+")
		if rest == "@" {
			rest = "Space"
		}
		return "^" + strings.ToUpper(rest)
	case strings.HasPrefix(key, "alt+"):
		return "M-" + strings.ToUpper(strings.TrimPrefix(key, "alt+"))
	default:
		return key
	}
}

func (m Model) renderTooltipBar() string {
	chip := func(key, action string) string {
		k := lipgloss.NewStyle().
			Background(activeTheme.Accent).
			Foreground(activeTheme.AccentFg).
			Bold(true).
			Padding(0, 1).
			Render(key)
		if action == "" {
			return k
		}
		a := lipgloss.NewStyle().
			Background(activeTheme.BlurredBg).
			Foreground(activeTheme.TextPrimary).
			Padding(0, 1).
			Render(action)
		return k + a
	}

	withStatus := func(bar string) string {
		if m.statusMsg != "" {
			bar += lipgloss.NewStyle().Foreground(activeTheme.Cursor).Render("  " + m.statusMsg)
		}
		return lipgloss.NewStyle().Width(m.width).Background(activeTheme.StatusBg).Render(bar)
	}

	// Config overlay: show config-specific hints.
	if m.showConfig {
		bar := strings.Join([]string{chip("↑↓", "select"), chip("←→", "change"), chip("Tab", "section"), chip("Esc", "save & close")}, " ")
		return lipgloss.NewStyle().Width(m.width).Background(activeTheme.StatusBg).Render(bar)
	}

	// Pane picker mode.
	if m.panePicker {
		total := len(m.splits) + 1
		bar := strings.Join([]string{chip("←/→", "select pane"), chip("↵", "confirm"), chip("Esc", "cancel")}, " ")
		bar += lipgloss.NewStyle().Foreground(activeTheme.Cursor).
			Render(strconv.Itoa(m.pickerIdx+1) + "/" + strconv.Itoa(total))
		return lipgloss.NewStyle().Width(m.width).Background(activeTheme.StatusBg).Render(bar)
	}

	// Help view: show scroll and close hints only.
	if m.activePane == paneMain && len(m.splits) > 0 && m.splits[m.activeSplit].activeView == viewHelp {
		bar := strings.Join([]string{chip("j/k", "scroll"), chip("g/G", "top/bottom"), chip("Esc", "close help")}, " ")
		return lipgloss.NewStyle().Width(m.width).Background(activeTheme.StatusBg).Render(bar)
	}

	// Edit mode: show edit-specific hints.
	if m.activePane == paneMain && len(m.splits) > 0 && m.splits[m.activeSplit].activeView == viewEdit {
		bar := strings.Join([]string{chip(keyChipLabel(m.cfg.Keymap.Save), "save"), chip(keyChipLabel(m.cfg.Keymap.Palette), "command"), chip("Tab", "cycle fields"), chip("Esc", "cancel"), chip("^C", "quit")}, " ")
		return lipgloss.NewStyle().Width(m.width).Background(activeTheme.StatusBg).Render(bar)
	}

	// Group label style for SHIFT+ / CTRL+ prefixes.
	grp := func(s string) string {
		return lipgloss.NewStyle().
			Foreground(activeTheme.TextMuted).
			Render(s)
	}

	shiftChips := strings.Join([]string{
		chip("N", "new"),
		chip("A", "archive"),
		chip("M", "move"),
		chip("O", "open"),
		chip("T", "insert"),
		chip("P", "add project"),
	}, " ")

	ctrlParts := []string{chip(keyChipLabel(m.cfg.Keymap.PanePicker), "panes"), chip(keyChipLabel(m.cfg.Keymap.Quit), "quit")}
	if len(m.splits) > 1 {
		ctrlParts = append(ctrlParts, chip(keyChipLabel(m.cfg.Keymap.NextPane), "next pane"))
	}
	if len(m.undoStack) > 0 {
		ctrlParts = append(ctrlParts, chip(keyChipLabel(m.cfg.Keymap.Undo), "undo"))
	}
	if len(m.redoStack) > 0 {
		ctrlParts = append(ctrlParts, chip(keyChipLabel(m.cfg.Keymap.Redo), "redo"))
	}
	ctrlChips := strings.Join(ctrlParts, " ")

	bar := chip(":", "command") + "  " + chip("?", "help") + "   " +
		grp("SHIFT +") + " " + shiftChips + "   " +
		grp("CTRL +") + " " + ctrlChips
	return withStatus(bar)
}

// updateProjectDetail forwards a key to the project detail pane and, if it
// completed a bridge entry, records the history event and reloads the project.
func (m *Model) updateProjectDetail(sp *splitPane, msg tea.KeyMsg) {
	pd := sp.projectDetail.update(msg)
	if pd.bridgeDone {
		p := pd.project
		_ = m.vault.Projects.AddHistory(p.Name, vault.HistoryEntry{
			Timestamp: timeNow(),
			Kind:      vault.HistoryKindNote,
			Message:   pd.bridgeMessage,
		})
		pd.bridgeDone = false
		pd.bridgeMessage = ""
		// Reload project to pick up updated history.
		if updated, ok := m.vault.Projects.Get(p.Name); ok {
			pd.project = updated
		}
	}
	sp.projectDetail = pd
}

func (m *Model) showSectionLanding(state vault.NoteState, isTemplates bool) {
	if state == vault.StateProjects && !isTemplates {
		allNotes, _ := m.vault.ListAll()
		m.splits[m.activeSplit].allProjects = m.vault.Projects.ListActive()
		m.splits[m.activeSplit].notes = allNotes
		m.splits[m.activeSplit].activeView = viewProjectsOverview
	} else {
		var notes []*vault.Note
		if isTemplates {
			notes, _ = m.vault.ListByTag("template")
		} else {
			notes, _ = m.vault.ListByState(state)
		}
		m.splits[m.activeSplit].sectionLanding = sectionLandingPane{
			state:       state,
			isTemplates: isTemplates,
			notes:       notes,
			count:       len(notes),
		}
		m.splits[m.activeSplit].activeView = viewSectionLanding
	}
}

func (m *Model) handleUndo() {
	if len(m.undoStack) == 0 {
		m.statusMsg = "nothing to undo"
		return
	}
	rec := m.undoStack[len(m.undoStack)-1]
	m.undoStack = m.undoStack[:len(m.undoStack)-1]
	old := rec.oldNote
	if err := m.vault.Save(&old); err != nil {
		m.statusMsg = "undo error: " + err.Error()
		return
	}
	m.index.Upsert(&old)
	l := m.computeLayout()
	for i := range m.splits {
		if m.splits[i].viewer.note != nil && m.splits[i].viewer.note.ID == old.ID {
			m.splits[i].viewer = m.splits[i].viewer.withNote(&old)
			m.splits[i].viewer = m.splits[i].viewer.preRender(l.paneWidth, m.titleSet)
		}
	}
	m.redoStack = append(m.redoStack, rec)
	m.statusMsg = "undone"
}

func (m *Model) handleRedo() {
	if len(m.redoStack) == 0 {
		m.statusMsg = "nothing to redo"
		return
	}
	rec := m.redoStack[len(m.redoStack)-1]
	m.redoStack = m.redoStack[:len(m.redoStack)-1]
	next := rec.newNote
	if err := m.vault.Save(&next); err != nil {
		m.statusMsg = "redo error: " + err.Error()
		return
	}
	m.index.Upsert(&next)
	l := m.computeLayout()
	for i := range m.splits {
		if m.splits[i].viewer.note != nil && m.splits[i].viewer.note.ID == next.ID {
			m.splits[i].viewer = m.splits[i].viewer.withNote(&next)
			m.splits[i].viewer = m.splits[i].viewer.preRender(l.paneWidth, m.titleSet)
		}
	}
	m.undoStack = append(m.undoStack, rec)
	m.statusMsg = "redone"
}

// variableNames returns the configured variable names, sorted, for palette autosuggest.
func (m *Model) variableNames() []string {
	names := make([]string, 0, len(m.cfg.Variables))
	for name := range m.cfg.Variables {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func buildTitleSet(v *vault.Vault) map[string]bool {
	all, _ := v.ListAll()
	set := make(map[string]bool, len(all))
	for _, n := range all {
		set[strings.ToLower(n.Title)] = true
	}
	return set
}

func capitalize(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

type statusMsg string
type notesLoadedMsg struct{ notes []*vault.Note }

// sortedNotes returns all vault notes sorted by Updated descending, for palette completion.
func sortedNotes(v *vault.Vault) []*vault.Note {
	all, _ := v.ListAll()
	sort.Slice(all, func(i, j int) bool {
		return all[i].Updated.After(all[j].Updated)
	})
	return all
}
type countsRefreshedMsg struct {
	counts        map[vault.NoteState]int
	templateCount int
}
