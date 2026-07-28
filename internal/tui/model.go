package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"pkm/internal/gitlab"
	"pkm/internal/index"
	"pkm/internal/vault"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
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
	viewTasksOverview
	viewTrash
	viewIssueDetail
)

type undoRecord struct {
	oldNote vault.Note
	newNote vault.Note
	// isDelete marks a :delete's undo record (#1): oldNote == newNote here
	// (there's no "new" content, only "gone"). Undo/redo for these must
	// also manage the trash file/sidecar entry, not just Save — a generic
	// Save-based undo would recreate the note while leaving an orphaned
	// copy behind in trash, and a generic Save-based redo would recreate
	// the note instead of re-trashing it. See handleUndo/handleRedo.
	isDelete bool
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
	indexing   bool
	panePicker bool
	pickerIdx  int // 0 = sidebar, 1..n = splits[0..n-1]

	pendingMoveNote *vault.Note // note awaiting project assignment before move to projects

	undoStack []undoRecord
	redoStack []undoRecord

	cfg        AppConfig
	showConfig bool
	configView configPane

	showImport bool
	importView importPane

	showExport bool
	exportView exportPane

	titleSet map[string]bool // lowercase note titles → used for link existence checks

	// searchResults is non-nil exactly when m.noteList currently displays a
	// :search (or :open's search-fallback) result set — opening a note from
	// that list marks the split so "back" can restore the results (see
	// splitPane.searchReturn) instead of falling through to whatever was
	// open before the search. Sticky by design: only cleared by a fresh
	// search or notesLoadedMsg. Correct today because nothing else populates
	// m.noteList; if a future "browse as list" path is added, it must clear
	// this field or its notes will be wrongly treated as search results.
	searchResults []*vault.Note

	issuesCache   gitlab.Cache
	issuesFetched bool
	issuesSyncing bool
	gitlabToken   string
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
//
// The Project field is validated (EnsureProject, which enforces the
// max-active-projects limit) before anything is mutated or saved: a
// max-reached error must leave both the draft and the on-disk note
// untouched and keep the editor open, rather than silently losing the
// user's edits.
func (m *Model) commitEditorDraft(sp *splitPane) {
	n := sp.editor.note
	editRawLine := sp.editor.ta.Line() // #22: where the draft's cursor was, for the return to view mode
	newProject := strings.TrimSpace(sp.editor.projInput.Value())
	oldProject := n.Project

	if newProject != "" && newProject != oldProject {
		if _, err := m.vault.Projects.EnsureProject(newProject); err != nil {
			m.statusMsg = "error: " + err.Error()
			return
		}
	}

	oldNote := *n // snapshot before mutation
	n.Body = sp.editor.ta.Value()
	n.State = vault.AllStates[sp.editor.stateIdx]
	n.Tags = parseTags(sp.editor.tagsInput.Value())

	var saveErr error
	switch {
	case newProject == oldProject:
		n.Project = newProject
		saveErr = m.vault.Save(n)
		if saveErr == nil {
			m.index.Upsert(n)
		}
	case newProject != "":
		saveErr = m.assignProjectToNote(n, newProject)
	default: // project cleared (oldProject was non-empty)
		m.recordDetach(n) // n.Project still holds oldProject here
		n.Project = ""
		saveErr = m.vault.Save(n)
		if saveErr == nil {
			m.index.Upsert(n)
		}
	}

	if saveErr != nil {
		m.statusMsg = "save error: " + saveErr.Error()
	} else {
		rec := undoRecord{oldNote: oldNote, newNote: *n}
		m.undoStack = append(m.undoStack, rec)
		if len(m.undoStack) > 20 {
			m.undoStack = m.undoStack[1:]
		}
		m.redoStack = nil
		sp.viewer = sp.viewer.withNote(n)
		l := m.computeLayout()
		sp.viewer = sp.viewer.preRender(l.paneWidth, m.titleSet)
		// #22: land back on the rendered line the draft's cursor was on,
		// instead of withNote's reset to the top.
		sp.viewer.cursorRow = sp.viewer.renderedLineForRaw(editRawLine)
		sp.viewer.cursorCol = 0
		sp.viewer = sp.viewer.followCursor(l.contentHeight)
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
	m.gitlabToken = os.Getenv("PKM_GITLAB_TOKEN")
	m.issuesCache = gitlab.LoadCache(filepath.Join(v.Root, ".pkm"))

	// Load config first (theme, sidebar width, etc.).
	cfg := loadConfig(v)
	m.cfg = cfg
	m.sidebar.showTasksNav = cfg.ShowTasksNav
	m.sidebar.showTemplatesNav = cfg.ShowTemplatesNav

	// #1: purge trash entries past retention, once at process start —
	// mirrors the startup index-validation scan (CLAUDE.md "Index
	// Architecture"), no timer/background polling.
	v.PurgeExpired(cfg.TrashRetentionDays)

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
	return tea.Batch(m.sidebar.init(), tea.Sequence(startReindexCmd, reindexCmd(m.vault, m.index)), tickCmd())
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
		switch msg.Action {
		case tea.MouseActionPress:
			switch msg.Button {
			case tea.MouseButtonLeft:
				if cmd := m.handleMouseClick(msg.X, msg.Y); cmd != nil {
					cmds = append(cmds, cmd)
				}
			case tea.MouseButtonWheelUp, tea.MouseButtonWheelDown:
				m.handleMouseWheel(msg.X, msg.Button)
			}
		case tea.MouseActionMotion:
			m.handleMouseDrag(msg.X, msg.Y)
		case tea.MouseActionRelease:
			if cmd := m.handleMouseRelease(); cmd != nil {
				cmds = append(cmds, cmd)
			}
		}

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			// #2: Ctrl+C copies an active view-mode text selection instead of
			// the usual cancel-to-Esc remap below — but only then; this check
			// must run before the remap, not after, or the selection case
			// would never reach the viewer (same dispatch-order trap as the
			// Ctrl+L line-ops chord in #10).
			selecting := m.activePane == paneMain && len(m.splits) > 0 &&
				m.splits[m.activeSplit].activeView == viewNote && m.splits[m.activeSplit].viewer.selActive
			if !selecting {
				msg = tea.KeyMsg{Type: tea.KeyEsc}
			}
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
				// :search is special-cased: if the user arrowed to a dropdown
				// hit before pressing Enter, open it directly instead of
				// running :search as a text command (which would show the
				// full results view — see cmdSearch). Populate the results
				// list first (same query) so back-navigation out of the note
				// still lands on the results, exactly as if :search had run.
				if n, ok := m.palette.navigatedSearchNote(); ok {
					query := strings.TrimSpace(m.palette.inputAfterVerb())
					all, _ := m.vault.ListAll()
					results := fuzzySearchNotes(query, all)
					m.noteList = m.noteList.withNotes(results)
					m.searchResults = results
					sp := &m.splits[m.activeSplit]
					sp.openNoteFromSearch(n, results)
					l := m.computeLayout()
					sp.viewer = sp.viewer.preRender(l.paneWidth, m.titleSet)
					m.statusMsg = "opened: " + n.Title
					m.showPalette = false
					m.resizeOpenEditors(m.computeLayout())
					return m, tea.Batch(cmds...)
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
				if sp.editor.lineOpStatus != "" {
					m.statusMsg = sp.editor.lineOpStatus
					sp.editor.lineOpStatus = ""
				}
				if sp.editor.saved {
					m.commitEditorDraft(sp)
				} else if sp.editor.cancelled {
					if sp.editor.dirty() {
						m.commitEditorDraft(sp)
					} else {
						sp.editor = editPane{}
						sp.activeView = viewNote
					}
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
				m.cfg.GitLabURL = m.configView.gitlabURL
				m.cfg.GitLabProjects = append([]string(nil), m.configView.gitlabProjects...)
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
			case secIssues:
				m.configView = m.configView.updateIssues(msg)
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

		// Import overlay captures all input.
		if m.showImport {
			m.importView = m.importView.update(msg)
			if m.importView.cancelled {
				m.showImport = false
				return m, nil
			}
			if m.importView.confirmed {
				m.runImport()
				if !m.showImport {
					m.resizeOpenEditors(m.computeLayout())
				}
			}
			return m, nil
		}

		// Export overlay captures all input.
		if m.showExport {
			m.exportView = m.exportView.update(msg)
			if m.exportView.cancelled {
				m.showExport = false
				return m, nil
			}
			if m.exportView.confirmed {
				m.runExport()
				if !m.showExport {
					m.resizeOpenEditors(m.computeLayout())
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

		case "D":
			m.showPalette = true
			input := "delete "
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

		case "I":
			m.showImport = true
			m.importView = newImportPane()
			return m, nil

		case "e":
			sp := &m.splits[m.activeSplit]
			if sp.activeView == viewNote && sp.viewer.note != nil {
				l := m.computeLayout()
				startLine := sp.viewer.rawLineAt(sp.viewer.cursorRow) // #22: resume at the same position, not line 0
				var cmd tea.Cmd
				sp.editor, cmd = newEditPane(sp.viewer.note, l.paneWidth, l.contentHeight, sortedNotes(m.vault), m.vault.Projects.ActiveNames(), m.cfg.LineNumbers, m.cfg.Keymap.Save, startLine)
				sp.activeView = viewEdit
				m.activePane = paneMain
				return m, cmd
			}

		case "tab", "shift+tab":
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
						if n.Project == p.Name {
							pNotes = append(pNotes, n)
						}
					}
					m.splits[m.activeSplit].projectDetail = newProjectDetailPane(p, pNotes)
					m.splits[m.activeSplit].activeView = viewProjectDetail
					m.activePane = paneMain
				} else if m.sidebar.selectedTasks {
					m.sidebar.selectedTasks = false
					if cmd := m.openTasksOverview(); cmd != nil {
						cmds = append(cmds, cmd)
					}
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
					if m.searchResults != nil {
						sp.openNoteFromSearch(m.noteList.chosen, m.searchResults)
					} else {
						sp.openNote(m.noteList.chosen)
					}
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
					l := m.computeLayout()
					var cmd tea.Cmd
					sp.viewer, cmd = sp.viewer.update(msg, l.contentHeight)
					cmds = append(cmds, cmd)
					if sp.viewer.back {
						sp.viewer.back = false
						if sp.back() {
							l := m.computeLayout()
							sp.viewer = sp.viewer.preRender(l.paneWidth, m.titleSet)
						} else if sp.searchReturn != nil {
							m.noteList = m.noteList.withNotes(sp.searchReturn)
							m.searchResults = sp.searchReturn
							sp.activeView = viewList
						} else {
							sp.activeView = viewList
						}
					}
					if target := sp.viewer.pendingLinkOpen; target != "" {
						sp.viewer.pendingLinkOpen = ""
						m.openOrCreateNote(target)
					}
					if sp.viewer.pendingCheckboxRaw >= 0 {
						raw := sp.viewer.pendingCheckboxRaw
						sp.viewer.pendingCheckboxRaw = -1
						m.applyCheckboxToggle(sp, raw)
					}
					if content := sp.viewer.pendingCodeCopy; content != "" {
						sp.viewer.pendingCodeCopy = ""
						cmds = append(cmds, copyToClipboardCmd(content))
						m.statusMsg = "copied code block"
					}
					if text := sp.viewer.pendingCopyText; text != "" {
						sp.viewer.pendingCopyText = ""
						cmds = append(cmds, copyToClipboardCmd(text))
						// OSC 52 silently truncates past a per-terminal size
						// limit — reporting the count makes that visible
						// rather than a large Ctrl+A copy failing invisibly.
						m.statusMsg = fmt.Sprintf("copied %d characters", len([]rune(text)))
					}
					if sp.viewer.pendingFoldRaw >= 0 {
						raw := sp.viewer.pendingFoldRaw
						collapse := sp.viewer.pendingFoldCollapse
						sp.viewer.pendingFoldRaw = -1
						m.applyFold(sp, raw, collapse)
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
			case viewTasksOverview:
				switch msg.String() {
				case "esc", "backspace":
					sp.activeView = viewList
				case "j", "down":
					if sp.taskCursorRow < len(sp.taskRows)-1 {
						sp.taskCursorRow++
					}
					m.followTaskCursor(sp)
				case "k", "up":
					if sp.taskCursorRow > 0 {
						sp.taskCursorRow--
					}
					m.followTaskCursor(sp)
				case "g":
					sp.taskCursorRow = 0
					sp.taskScrollOff = 0
				case "G":
					sp.taskCursorRow = len(sp.taskRows) - 1
					m.followTaskCursor(sp)
				case "r":
					if m.gitlabToken != "" && len(m.cfg.GitLabProjects) > 0 && !m.issuesSyncing {
						m.issuesSyncing = true
						m.statusMsg = "syncing issues…"
						cmds = append(cmds, fetchIssuesCmd(
							gitlab.NewClient(m.cfg.GitLabURL, m.gitlabToken),
							m.cfg.GitLabProjects,
						))
					}
				case "enter":
					if sp.taskCursorRow >= 0 && sp.taskCursorRow < len(sp.taskRows) {
						row := sp.taskRows[sp.taskCursorRow]
						if row.task != nil {
							m.openOrCreateNote(row.task.note.Title)
						} else if row.issue != nil {
							issue := *row.issue
							sp.issueDetail = issueDetailPane{
								project: row.issueProject,
								issue:   &issue,
								loading: issue.UserNotesCount > 0 && m.gitlabToken != "",
							}
							sp.activeView = viewIssueDetail
							if sp.issueDetail.loading {
								cmds = append(cmds, fetchCommentsCmd(
									gitlab.NewClient(m.cfg.GitLabURL, m.gitlabToken),
									row.issueProject, issue.IID,
								))
							}
						}
					}
				}
			case viewIssueDetail:
				switch msg.String() {
				case "esc", "backspace":
					sp.activeView = viewTasksOverview
				case "j", "down":
					sp.issueDetail.scrollOff++
				case "k", "up":
					sp.issueDetail.scrollOff = max(0, sp.issueDetail.scrollOff-1)
				case "g":
					sp.issueDetail.scrollOff = 0
				case "G":
					l := m.computeLayout()
					lines := strings.Split(strings.TrimRight(sp.issueDetail.rendered, "\n"), "\n")
					sp.issueDetail.scrollOff = max(0, len(lines)-l.contentHeight)
				}
			case viewTrash:
				// Any key other than a repeated "d" on the same row cancels
				// a pending permanent-delete confirm (footer line, not a
				// modal — same convention as :export's overwrite confirm).
				if msg.String() != "d" && sp.trashConfirmID != "" {
					sp.trashConfirmID = ""
				}
				switch msg.String() {
				case "esc", "backspace":
					sp.activeView = viewList
				case "j", "down":
					if sp.trashCursorRow < len(sp.trashRows)-1 {
						sp.trashCursorRow++
					}
					m.followTrashCursor(sp)
				case "k", "up":
					if sp.trashCursorRow > 0 {
						sp.trashCursorRow--
					}
					m.followTrashCursor(sp)
				case "g":
					sp.trashCursorRow = 0
					sp.trashScrollOff = 0
				case "G":
					sp.trashCursorRow = len(sp.trashRows) - 1
					m.followTrashCursor(sp)
				case "enter":
					m.restoreTrashEntry(sp)
				case "d":
					if sp.trashCursorRow >= 0 && sp.trashCursorRow < len(sp.trashRows) {
						id := sp.trashRows[sp.trashCursorRow].ID
						if sp.trashConfirmID == id {
							m.permanentlyDeleteTrashEntry(sp)
						} else {
							sp.trashConfirmID = id
						}
					}
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
		m.sidebar.refreshNotesPreservingCursor()
		if msg.note != nil {
			l := m.computeLayout()
			for i := range m.splits {
				if m.splits[i].viewer.note != nil && m.splits[i].viewer.note.ID == msg.note.ID {
					row, col, scroll := m.splits[i].viewer.cursorRow, m.splits[i].viewer.cursorCol, m.splits[i].viewer.scrollOff
					folded := m.splits[i].viewer.folded
					m.splits[i].viewer = m.splits[i].viewer.withNote(msg.note)
					m.splits[i].viewer.cursorRow, m.splits[i].viewer.cursorCol, m.splits[i].viewer.scrollOff = row, col, scroll
					m.splits[i].viewer.folded = folded
					m.splits[i].viewer = m.splits[i].viewer.preRender(l.paneWidth, m.titleSet)
				}
			}
		}

	case reindexRequestedMsg:
		if !m.indexing {
			m.indexing = true
			m.statusMsg = "indexing…"
			cmds = append(cmds, reindexCmd(m.vault, m.index))
		}

	case reindexStartedMsg:
		m.indexing = true
		m.statusMsg = "indexing…"

	case reindexFinishedMsg:
		m.indexing = false
		if msg.err != nil {
			m.statusMsg = "indexing failed: " + msg.err.Error() + " (run :reindex to retry)"
			break
		}
		m.titleSet = buildTitleSet(m.vault)
		counts, _ := m.index.CountByState()
		m.sidebar = m.sidebar.withCounts(counts)
		m.sidebar.refreshNotesPreservingCursor()
		m.statusMsg = fmt.Sprintf("indexed %d notes", len(msg.notes))

	case issuesFetchedMsg:
		m.issuesSyncing = false
		if m.issuesCache.Projects == nil {
			m.issuesCache.Projects = map[string][]gitlab.Issue{}
		}
		for project, issues := range msg.projects {
			m.issuesCache.Projects[project] = issues
		}
		m.issuesCache.FetchedAt = msg.fetchedAt
		m.issuesCache.Version = 1
		_ = gitlab.SaveCache(filepath.Join(m.vault.Root, ".pkm"), m.issuesCache)
		if len(msg.errs) == 0 {
			m.statusMsg = "issues synced"
		} else {
			var first error
			for _, project := range m.cfg.GitLabProjects {
				if err := msg.errs[project]; err != nil {
					first = err
					break
				}
			}
			m.statusMsg = fmt.Sprintf("issues: %d repo(s) failed: %v", len(msg.errs), first)
		}
		for i := range m.splits {
			if m.splits[i].activeView == viewTasksOverview {
				m.splits[i].taskRows = append(buildTaskOverviewRows(m.vault),
					buildIssueRows(m.cfg, m.issuesCache, m.gitlabToken)...)
				if m.splits[i].taskCursorRow >= len(m.splits[i].taskRows) {
					m.splits[i].taskCursorRow = max(0, len(m.splits[i].taskRows)-1)
				}
			}
		}

	case issueCommentsMsg:
		for i := range m.splits {
			detail := &m.splits[i].issueDetail
			if m.splits[i].activeView != viewIssueDetail || detail.issue == nil ||
				detail.project != msg.project || detail.issue.IID != msg.iid {
				continue
			}
			detail.loading = false
			detail.comments = msg.comments
			if msg.err != nil {
				detail.commentsErr = msg.err.Error()
			}
			detail.rendered = ""
		}

	case statusMsg:
		m.statusMsg = string(msg)

	case notesLoadedMsg:
		m.noteList = m.noteList.withNotes(msg.notes)
		m.searchResults = nil
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
	case cfgItemShowTasksNav:
		m.cfg.ShowTasksNav = valueIdx == 0
		m.sidebar.showTasksNav = m.cfg.ShowTasksNav
		m.sidebar.clampCursor()
	case cfgItemShowTemplatesNav:
		m.cfg.ShowTemplatesNav = valueIdx == 0
		m.sidebar.showTemplatesNav = m.cfg.ShowTemplatesNav
		m.sidebar.clampCursor()
	case cfgItemTrashRetention:
		if valueIdx >= 0 && valueIdx < len(trashRetentionDaysOptions) {
			m.cfg.TrashRetentionDays = trashRetentionDaysOptions[valueIdx]
		}
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
	m.sidebar.showTasksNav = cfg.ShowTasksNav
	m.sidebar.showTemplatesNav = cfg.ShowTemplatesNav
	m.sidebar.clampCursor()
	if m.showConfig {
		m.configView = newConfigPane(m.cfg)
	}
	m.resizeOpenEditors(m.computeLayout())
}

func (m *Model) bustViewerCaches() {
	for i := range m.splits {
		m.splits[i].viewer.rendered = ""
		m.splits[i].viewer.renderWidth = 0
		m.splits[i].issueDetail.rendered = ""
		m.splits[i].issueDetail.renderWidth = 0
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

func (m *Model) handleMouseClick(x, y int) tea.Cmd {
	l := m.computeLayout()
	m.panePicker = false // any click exits picker mode

	// Click in sidebar area (x < sidebarWidth = outer width including border)
	if x < l.sidebarWidth {
		// y offsets with border: breadcrumb(0), top-border(1), SECTIONS(2), blank(3), items(4+)
		itemIdx := y - 4
		items := m.sidebar.items()
		if itemIdx < 0 || itemIdx >= len(items) {
			return nil
		}
		m.sidebar.cursor = itemIdx
		item := items[itemIdx]
		m.activePane = paneSidebar

		// Clicking the "▶"/"▼" glyph toggles expand/collapse only. Clicking
		// anywhere else on the row (the label) always shows the row's view,
		// leaving the expanded/collapsed state untouched.
		onGlyph := sidebarGlyphHit(item, x)

		if item.isSection {
			if onGlyph {
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
				}
				return nil
			}
			if item.isTemplates {
				m.sidebar.templatesActive = true
				m.showSectionLanding(m.sidebar.activeState, m.sidebar.templatesActive)
			} else if item.isTasks {
				m.sidebar.tasksActive = true
				m.sidebar.templatesActive = false
				return m.openTasksOverview()
			} else {
				m.sidebar.activeState = item.state
				m.sidebar.templatesActive = false
				m.sidebar.projectsActive = item.state == vault.StateProjects
				m.showSectionLanding(m.sidebar.activeState, m.sidebar.templatesActive)
			}
		} else if item.isFolderEntry {
			key := folderKey(item.state, item.folder)
			m.sidebar.expandedFolders[key] = !m.sidebar.expandedFolders[key]
			m.sidebar.activeState = item.state
			m.sidebar.projectsActive = false
			m.sidebar.templatesActive = false
			return nil
		} else if item.isProjectEntry {
			if onGlyph {
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
						if n.Project == item.project.Name {
							pNotes = append(pNotes, n)
						}
					}
					m.sidebar.projectNotesByName[item.project.Name] = pNotes
				}
				m.sidebar.activeProjectName = item.project.Name
				m.sidebar.projectsActive = true
				m.sidebar.templatesActive = false
				return nil
			}
			m.sidebar.activeProjectName = item.project.Name
			m.sidebar.projectsActive = true
			m.sidebar.templatesActive = false
			allNotes, _ := m.vault.ListAll()
			var pNotes []*vault.Note
			for _, n := range allNotes {
				if n.Project == item.project.Name {
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
		return nil
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
		paneX := mainX - clickedSplit*slotWidth // x within the clicked pane's outer box

		sp := &m.splits[m.activeSplit]
		if sp.activeView == viewList {
			// y offsets with border: breadcrumb(0), top-border(1), noteList-padding(2), notes(3+)
			noteIdx := y - 3
			if noteIdx >= 0 && noteIdx < len(m.noteList.notes) {
				m.noteList.cursor = noteIdx
				chosen := m.noteList.notes[noteIdx]
				if m.searchResults != nil {
					sp.openNoteFromSearch(chosen, m.searchResults)
				} else {
					sp.openNote(chosen)
				}
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
				return nil // click is in the sticky header or fold separator, no links there
			}
			bodyLine := (contentRow - sp.viewer.headerLineCount - 1) + sp.viewer.scrollOff
			if _, ok := sp.viewer.headingRawLineAt(bodyLine); ok {
				m.toggleFoldAt(sp, bodyLine)
			} else if target := sp.viewer.linkAtLine(bodyLine); target != "" {
				m.openOrCreateNote(target)
			} else if _, ok := sp.viewer.checkboxRawLineAt(bodyLine); ok {
				m.toggleCheckboxAt(sp, bodyLine)
			} else {
				// #2: a press over plain body text (not a link/checkbox/
				// heading, all handled above and unchanged) starts a
				// drag-select instead — see handleMouseDrag/handleMouseRelease
				// for how it's extended and finalized.
				col := paneX - 2 // -1 left border, -1 Padding(0,1)
				if col < 0 {
					col = 0
				}
				bodyLines := strings.Split(sp.viewer.rendered, "\n")
				if bodyLine >= 0 && bodyLine < len(bodyLines) {
					col = clampCol(col, bodyLines[bodyLine])
				}
				sp.viewer.selAnchorRow, sp.viewer.selAnchorCol = bodyLine, col
				sp.viewer.cursorRow, sp.viewer.cursorCol = bodyLine, col
				sp.viewer.selActive = false
				sp.viewer.dragging = true
			}
		}
	}
	return nil
}

// applyFold sets the fold state of the heading at rawLine (#20), forces a
// re-render (fold state isn't part of preRender's width-based cache key, so
// clearing m.rendered is what actually invalidates it), and clamps
// cursor/scroll into the possibly-shortened content afterward — a collapse
// can leave both pointing past the new end.
func (m *Model) applyFold(sp *splitPane, rawLine int, collapsed bool) {
	if sp.viewer.folded == nil {
		sp.viewer.folded = map[int]bool{}
	}
	sp.viewer.folded[rawLine] = collapsed
	sp.viewer.rendered = ""
	l := m.computeLayout()
	sp.viewer = sp.viewer.preRender(l.paneWidth, m.titleSet)
	sp.viewer = sp.viewer.clampScroll()
	sp.viewer = sp.viewer.followCursor(l.contentHeight)
}

// toggleFoldAt flips the fold state of the heading at a rendered body line,
// if any — the mouse-click path (#20); Left/Right on a heading set a fixed
// direction instead (see viewerModel.update's pendingFoldRaw handling below).
func (m *Model) toggleFoldAt(sp *splitPane, bodyLine int) {
	rawLine, ok := sp.viewer.headingRawLineAt(bodyLine)
	if !ok {
		return
	}
	m.applyFold(sp, rawLine, !sp.viewer.folded[rawLine])
}

// toggleCheckboxAt flips the "[ ]"/"[x]" checkbox on a rendered body line, if
// any, and saves the note. Shared by the view-mode mouse click above and the
// keyboard block cursor's Enter action.
// toggleCheckboxAt toggles the checkbox on a rendered body line (mouse click
// path), if any.
func (m *Model) toggleCheckboxAt(sp *splitPane, bodyLine int) {
	rawLine, ok := sp.viewer.checkboxRawLineAt(bodyLine)
	if !ok {
		return
	}
	m.applyCheckboxToggle(sp, rawLine)
}

// toggleCheckboxOnNote flips the checkbox at rawLine in n's body, saves,
// indexes, and records an undo entry. Shared by the note viewer's checkbox
// toggle (applyCheckboxToggle) and the Project Detail pane's task list
// toggle (toggleProjectDetailTask, #19).
func (m *Model) toggleCheckboxOnNote(n *vault.Note, rawLine int) bool {
	newBody, ok := toggleCheckboxLine(n.Body, rawLine)
	if !ok {
		return false
	}
	oldNote := *n
	n.Body = newBody
	if err := m.vault.Save(n); err != nil {
		m.statusMsg = "save error: " + err.Error()
		return false
	}
	m.undoStack = append(m.undoStack, undoRecord{oldNote: oldNote, newNote: *n})
	if len(m.undoStack) > 20 {
		m.undoStack = m.undoStack[1:]
	}
	m.redoStack = nil
	m.index.Upsert(n)
	return true
}

// applyCheckboxToggle flips the checkbox on a raw body line, saves, and
// re-renders — shared by the mouse click above and the keyboard cursor's
// Enter action. Preserves cursor/scroll position across the refresh.
func (m *Model) applyCheckboxToggle(sp *splitPane, rawLine int) {
	if sp.viewer.note == nil {
		return
	}
	n := sp.viewer.note
	if !m.toggleCheckboxOnNote(n, rawLine) {
		return
	}

	row, col, scroll := sp.viewer.cursorRow, sp.viewer.cursorCol, sp.viewer.scrollOff
	folded := sp.viewer.folded
	sp.viewer = sp.viewer.withNote(n)
	sp.viewer.cursorRow, sp.viewer.cursorCol, sp.viewer.scrollOff = row, col, scroll
	sp.viewer.folded = folded
	l := m.computeLayout()
	sp.viewer = sp.viewer.preRender(l.paneWidth, m.titleSet)
}

// toggleProjectDetailTask toggles the checkbox for the task under the
// Project Detail pane's cursor and rebuilds its task rows from the updated
// note (#19).
func (m *Model) toggleProjectDetailTask(sp *splitPane) {
	pd := sp.projectDetail
	if pd.taskCursorRow < 0 || pd.taskCursorRow >= len(pd.taskRows) {
		return
	}
	row := pd.taskRows[pd.taskCursorRow]
	if row.task == nil {
		return
	}
	if !m.toggleCheckboxOnNote(row.task.note, row.task.rawLine) {
		return
	}
	sp.projectDetail.taskRows = projectTaskRows(pd.notes)
}

// handleMouseWheel scrolls whichever pane (sidebar or a main split) is under
// the cursor, without stealing focus from the currently active pane.
func (m *Model) handleMouseWheel(x int, button tea.MouseButton) {
	l := m.computeLayout()
	up := button == tea.MouseButtonWheelUp

	if x < l.sidebarWidth {
		items := m.sidebar.items()
		if up {
			if m.sidebar.cursor > 0 {
				m.sidebar.cursor--
			}
		} else if m.sidebar.cursor < len(items)-1 {
			m.sidebar.cursor++
		}
		return
	}

	if x < l.sidebarWidth+1 || l.paneWidth <= 0 {
		return
	}
	mainX := x - l.sidebarWidth - 1
	paneOuter := l.paneWidth + 2
	slotWidth := paneOuter + 1
	idx := mainX / slotWidth
	if idx >= len(m.splits) {
		idx = len(m.splits) - 1
	}
	sp := &m.splits[idx]

	switch sp.activeView {
	case viewNote:
		if up {
			if sp.viewer.scrollOff > 0 {
				sp.viewer.scrollOff--
			}
		} else {
			sp.viewer.scrollOff++
		}
	case viewList:
		if up {
			if m.noteList.cursor > 0 {
				m.noteList.cursor--
			}
		} else if m.noteList.cursor < len(m.noteList.notes)-1 {
			m.noteList.cursor++
		}
	case viewHelp:
		maxOff := HelpTotalLines() - l.contentHeight
		if maxOff < 0 {
			maxOff = 0
		}
		if up {
			if sp.helpScrollOff > 0 {
				sp.helpScrollOff--
			}
		} else if sp.helpScrollOff < maxOff {
			sp.helpScrollOff++
		}
	}
}

// handleMouseDrag extends the active split's selection while a mouse button
// is held (#2, MouseActionMotion — reported continuously during a drag by
// tea.WithMouseCellMotion() in cmd/pkm/main.go). No-op unless a press over
// plain body text (handleMouseClick's drag-start branch) is already in
// progress; a press on a link/checkbox/heading never sets dragging, so
// motion after one of those can't drag stale coordinates into a new note.
func (m *Model) handleMouseDrag(x, y int) {
	if len(m.splits) == 0 {
		return
	}
	sp := &m.splits[m.activeSplit]
	if !sp.viewer.dragging || sp.activeView != viewNote {
		return
	}
	l := m.computeLayout()
	paneOuter := l.paneWidth + 2
	slotWidth := paneOuter + 1
	paneX := x - l.sidebarWidth - 1 - m.activeSplit*slotWidth
	col := paneX - 2
	if col < 0 {
		col = 0
	}
	contentRow := y - 2
	bodyLine := (contentRow - sp.viewer.headerLineCount - 1) + sp.viewer.scrollOff
	if bodyLine < 0 {
		bodyLine = 0
	}
	bodyLines := strings.Split(sp.viewer.rendered, "\n")
	if len(bodyLines) == 0 {
		return
	}
	if bodyLine >= len(bodyLines) {
		bodyLine = len(bodyLines) - 1
	}
	col = clampCol(col, bodyLines[bodyLine])
	if bodyLine != sp.viewer.selAnchorRow || col != sp.viewer.selAnchorCol {
		sp.viewer.selActive = true
	}
	sp.viewer.cursorRow, sp.viewer.cursorCol = bodyLine, col
	sp.viewer = sp.viewer.followCursor(l.contentHeight)
}

// handleMouseRelease ends a drag-select and, if it produced a real
// selection, copies it automatically (#2) — unlike Ctrl+C, which requires an
// explicit keypress, mirroring how mouse selection behaves in other
// terminal apps.
func (m *Model) handleMouseRelease() tea.Cmd {
	if len(m.splits) == 0 {
		return nil
	}
	sp := &m.splits[m.activeSplit]
	if !sp.viewer.dragging {
		return nil
	}
	sp.viewer.dragging = false
	if !sp.viewer.selActive {
		return nil
	}
	text := sp.viewer.selectedRawText()
	if text == "" {
		return nil
	}
	m.statusMsg = fmt.Sprintf("copied %d characters", len([]rune(text)))
	return copyToClipboardCmd(text)
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
	} else if m.showImport {
		importInner := l.mainWidth - 2
		if importInner < 1 {
			importInner = 1
		}
		main = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(activeTheme.BorderFocus).
			Width(importInner).
			Height(l.contentHeight).
			Render(m.importView.render(importInner, l.contentHeight))
	} else if m.showExport {
		exportInner := l.mainWidth - 2
		if exportInner < 1 {
			exportInner = 1
		}
		main = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(activeTheme.BorderFocus).
			Width(exportInner).
			Height(l.contentHeight).
			Render(m.exportView.render(exportInner, l.contentHeight))
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
		case viewTasksOverview:
			content = renderTaskOverview(sp.taskRows, pi, l.contentHeight, sp.taskScrollOff, sp.taskCursorRow, focused)
		case viewTrash:
			content = renderTrash(sp.trashRows, m.cfg.TrashRetentionDays, pi, l.contentHeight, sp.trashScrollOff, sp.trashCursorRow, sp.trashConfirmID, focused)
		case viewIssueDetail:
			content = m.splits[i].issueDetail.render(pi, l.contentHeight)
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
			n := sp.viewer.note
			title += "  ›  " + capitalize(string(n.State))
			if n.State == vault.StateProjects && n.Project != "" {
				title += "  ›  " + n.Project
			}
			title += "  ›  " + n.Title
		}
	case viewProjectDetail:
		if sp.projectDetail.project != nil {
			title += "  ›  Projects  ›  " + sp.projectDetail.project.Name
		}
	case viewProjectsOverview:
		title += "  ›  Projects"
	case viewTasksOverview:
		title += "  ›  Tasks"
	case viewTrash:
		title += "  ›  Trash"
	case viewIssueDetail:
		title += "  ›  Tasks  ›  Issue"
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
	if lipgloss.Width(content) > m.width {
		content = xansi.Truncate(content, m.width, "")
	}

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

// fitTooltipBar guarantees the bottom hotkey/tooltip bar renders as exactly
// one line. lipgloss.Style.Width alone pads a short string but WORD-WRAPS a
// long one onto a second line instead of clipping it — at narrower terminal
// widths the full chip list doesn't fit, so the frame silently grew one row
// taller than the window height. A real terminal then has to scroll to show
// it, which desyncs every mouse click's Y coordinate from the row the app's
// layout math assumes (#18) — the visible symptom was clicks landing "about
// 1.5 lines" off the sidebar item they were meant to hit. Hard-truncating
// here, the same safety-net technique the editor/viewer footers already use
// (see editPane.footerText), keeps the frame exactly m.height rows tall.
func fitTooltipBar(bar string, width int) string {
	if lipgloss.Width(bar) > width {
		bar = xansi.Truncate(bar, width, "")
	}
	return lipgloss.NewStyle().Width(width).Background(activeTheme.StatusBg).Render(bar)
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
		return fitTooltipBar(bar, m.width)
	}

	// Config overlay: show config-specific hints.
	if m.showConfig {
		bar := strings.Join([]string{chip("↑↓", "select"), chip("←→", "change"), chip("Tab", "section"), chip("Esc", "save & close")}, " ")
		return fitTooltipBar(bar, m.width)
	}

	// Import overlay: show import-specific hints.
	if m.showImport {
		bar := strings.Join([]string{chip("Tab", "next field"), chip("Space", "toggle mode"), chip("Enter", "confirm/complete"), chip("Esc", "cancel")}, " ")
		return fitTooltipBar(bar, m.width)
	}

	// Export overlay: show export-specific hints.
	if m.showExport {
		bar := strings.Join([]string{chip("Tab", "next field"), chip("Enter", "confirm/complete"), chip("Esc", "cancel")}, " ")
		return fitTooltipBar(bar, m.width)
	}

	// Pane picker mode.
	if m.panePicker {
		total := len(m.splits) + 1
		bar := strings.Join([]string{chip("←/→", "select pane"), chip("↵", "confirm"), chip("Esc", "cancel")}, " ")
		bar += lipgloss.NewStyle().Foreground(activeTheme.Cursor).
			Render(strconv.Itoa(m.pickerIdx+1) + "/" + strconv.Itoa(total))
		return fitTooltipBar(bar, m.width)
	}

	// Help view: show scroll and close hints only.
	if m.activePane == paneMain && len(m.splits) > 0 && m.splits[m.activeSplit].activeView == viewHelp {
		bar := strings.Join([]string{chip("j/k", "scroll"), chip("g/G", "top/bottom"), chip("Esc", "close help")}, " ")
		return fitTooltipBar(bar, m.width)
	}

	// Task Overview: show cursor movement, open, and close hints.
	if m.activePane == paneMain && len(m.splits) > 0 && m.splits[m.activeSplit].activeView == viewTasksOverview {
		parts := []string{chip("j/k", "select"), chip("g/G", "top/bottom"), chip("Enter", "open")}
		if len(m.cfg.GitLabProjects) > 0 {
			parts = append(parts, chip("r", "sync issues"))
		}
		parts = append(parts, chip("Esc", "close"))
		bar := strings.Join(parts, " ")
		return fitTooltipBar(bar, m.width)
	}

	if m.activePane == paneMain && len(m.splits) > 0 && m.splits[m.activeSplit].activeView == viewIssueDetail {
		bar := strings.Join([]string{chip("j/k", "scroll"), chip("g/G", "top/bottom"), chip("Esc", "back")}, " ")
		return fitTooltipBar(bar, m.width)
	}

	// Edit mode: show edit-specific hints. Edit mode leans on Ctrl
	// shortcuts, so its group goes first (#3); Tab/Esc are unbound plain
	// keys, so they're an unlabeled group at the end.
	if m.activePane == paneMain && len(m.splits) > 0 && m.splits[m.activeSplit].activeView == viewEdit {
		ctrlChips := strings.Join([]string{
			chip(keyChipLabel(m.cfg.Keymap.Save), "save"),
			chip(keyChipLabel(m.cfg.Keymap.Palette), "command"),
			chip("^C", "quit"),
		}, " ")
		plainChips := strings.Join([]string{chip("Tab", "cycle fields"), chip("Esc", "save & close")}, " ")
		bar := joinGroups(m.labeledGroup("CTRL +", ctrlChips), plainChips)
		return fitTooltipBar(bar, m.width)
	}

	shiftChips := strings.Join([]string{
		chip("N", "new"),
		chip("A", "archive"),
		chip("D", "delete"),
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

	plainChips := chip(":", "command") + "  " + chip("?", "help")

	// This default bar covers every mode besides Edit and the modal
	// overlays above (List, Note Viewer, Trash, Project Detail, ...) —
	// loosely "View mode" in #3's terms, which leans on Shift shortcuts,
	// so that group goes first; the unbound ":"/"?" keys go last.
	bar := joinGroups(m.labeledGroup("SHIFT +", shiftChips), m.labeledGroup("CTRL +", ctrlChips), plainChips)
	return withStatus(bar)
}

// tooltipGroupMinWidth is the narrowest bar width at which the "SHIFT +"/
// "CTRL +" group labels are worth the screen space they cost (#3). Below
// it, labeledGroup drops the label text entirely and falls back to a flat,
// unlabeled chip list — same chips, same mode-dependent order, just no
// group header eating into room that matters more at this width.
const tooltipGroupMinWidth = 80

// labeledGroup pairs a group header with its chips, or — below
// tooltipGroupMinWidth — returns just the chips with no header (#3). Empty
// chips yield "" so joinGroups can drop the segment entirely.
func (m Model) labeledGroup(label, chips string) string {
	if chips == "" {
		return ""
	}
	if m.width < tooltipGroupMinWidth {
		return chips
	}
	return lipgloss.NewStyle().Foreground(activeTheme.TextMuted).Render(label) + " " + chips
}

// joinGroups lays out already-built group segments with a consistent gap,
// skipping any that collapsed to "" (labeledGroup with no chips).
func joinGroups(segments ...string) string {
	var parts []string
	for _, s := range segments {
		if s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, "   ")
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
	if sp.projectDetail.pendingToggle {
		sp.projectDetail.pendingToggle = false
		m.toggleProjectDetailTask(sp)
	}
	if sp.projectDetail.pendingOpen != "" {
		title := sp.projectDetail.pendingOpen
		sp.projectDetail.pendingOpen = ""
		m.openOrCreateNote(title)
	}
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

// openTasksOverview assembles issue #13's Task Overview (a fresh vault scan,
// per-view-open — there is no task index yet) and switches the active split
// to it. Shared by the :tasks command, the sidebar's Tasks row, and its
// mouse-click equivalent.
func (m *Model) openTasksOverview() tea.Cmd {
	sp := &m.splits[m.activeSplit]
	sp.taskRows = append(buildTaskOverviewRows(m.vault), buildIssueRows(m.cfg, m.issuesCache, m.gitlabToken)...)
	sp.taskScrollOff = 0
	sp.taskCursorRow = 0
	sp.activeView = viewTasksOverview
	m.activePane = paneMain
	if m.gitlabToken != "" && len(m.cfg.GitLabProjects) > 0 &&
		!m.issuesFetched && !m.issuesSyncing {
		m.issuesFetched = true
		m.issuesSyncing = true
		m.statusMsg = "syncing issues…"
		return fetchIssuesCmd(gitlab.NewClient(m.cfg.GitLabURL, m.gitlabToken), m.cfg.GitLabProjects)
	}
	return nil
}

// followTaskCursor adjusts sp.taskScrollOff so taskCursorRow stays within
// the visible window, mirroring viewerModel.followCursor.
func (m *Model) followTaskCursor(sp *splitPane) {
	rows := m.computeLayout().contentHeight
	if sp.taskCursorRow < sp.taskScrollOff {
		sp.taskScrollOff = sp.taskCursorRow
	} else if sp.taskCursorRow >= sp.taskScrollOff+rows {
		sp.taskScrollOff = sp.taskCursorRow - rows + 1
	}
	if sp.taskScrollOff < 0 {
		sp.taskScrollOff = 0
	}
}

// openTrashView assembles #1's trash list (a fresh sidecar read, same
// per-view-open convention as openTasksOverview — there is no persistent
// trash index either) and switches the active split to it.
func (m *Model) openTrashView() {
	sp := &m.splits[m.activeSplit]
	entries, _ := m.vault.ListTrash()
	sp.trashRows = entries
	sp.trashScrollOff = 0
	sp.trashCursorRow = 0
	sp.trashConfirmID = ""
	sp.activeView = viewTrash
	m.activePane = paneMain
}

// followTrashCursor adjusts sp.trashScrollOff so trashCursorRow stays
// within the visible window, mirroring followTaskCursor.
func (m *Model) followTrashCursor(sp *splitPane) {
	rows := m.computeLayout().contentHeight
	if sp.trashCursorRow < sp.trashScrollOff {
		sp.trashScrollOff = sp.trashCursorRow
	} else if sp.trashCursorRow >= sp.trashScrollOff+rows {
		sp.trashScrollOff = sp.trashCursorRow - rows + 1
	}
	if sp.trashScrollOff < 0 {
		sp.trashScrollOff = 0
	}
}

// restoreTrashEntry restores the trash row under the cursor, reindexes the
// recovered note, and refreshes the list in place (rather than reopening
// the view) so the cursor position stays sensible after a row disappears.
func (m *Model) restoreTrashEntry(sp *splitPane) {
	if sp.trashCursorRow < 0 || sp.trashCursorRow >= len(sp.trashRows) {
		return
	}
	entry := sp.trashRows[sp.trashCursorRow]
	n, err := m.vault.Restore(entry)
	if err != nil {
		m.statusMsg = "restore error: " + err.Error()
		return
	}
	m.index.Upsert(n)
	m.titleSet[strings.ToLower(n.Title)] = true
	refreshCounts(m)

	entries, _ := m.vault.ListTrash()
	sp.trashRows = entries
	sp.trashConfirmID = ""
	if sp.trashCursorRow >= len(sp.trashRows) {
		sp.trashCursorRow = max(0, len(sp.trashRows)-1)
	}
	m.statusMsg = "restored: " + n.Title
}

// permanentlyDeleteTrashEntry removes the trash row under the cursor for
// good — only called once the footer confirm ("press d again") has already
// matched this row's ID, per cmdDelete's own "safety net, not a modal"
// convention used throughout this app.
func (m *Model) permanentlyDeleteTrashEntry(sp *splitPane) {
	if sp.trashCursorRow < 0 || sp.trashCursorRow >= len(sp.trashRows) {
		return
	}
	entry := sp.trashRows[sp.trashCursorRow]
	if err := m.vault.RemoveTrashEntry(entry.ID); err != nil {
		m.statusMsg = "delete error: " + err.Error()
		return
	}
	sp.trashConfirmID = ""
	entries, _ := m.vault.ListTrash()
	sp.trashRows = entries
	if sp.trashCursorRow >= len(sp.trashRows) {
		sp.trashCursorRow = max(0, len(sp.trashRows)-1)
	}
	m.statusMsg = "permanently deleted: " + entry.Title
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
	trashWarning := ""
	if rec.isDelete {
		// #1: Save above just recreated the note at its original path; the
		// trashed copy and its sidecar entry are now an orphan (the same
		// note existing twice) unless removed here too.
		if err := m.vault.RemoveTrashEntry(old.ID); err != nil {
			trashWarning = "undo warning: trash cleanup failed: " + err.Error()
		}
	}
	m.index.Upsert(&old)
	m.titleSet[strings.ToLower(old.Title)] = true
	l := m.computeLayout()
	for i := range m.splits {
		if m.splits[i].viewer.note != nil && m.splits[i].viewer.note.ID == old.ID {
			m.splits[i].viewer = m.splits[i].viewer.withNote(&old)
			m.splits[i].viewer = m.splits[i].viewer.preRender(l.paneWidth, m.titleSet)
		}
	}
	m.redoStack = append(m.redoStack, rec)
	if trashWarning != "" {
		m.statusMsg = trashWarning
	} else {
		m.statusMsg = "undone"
	}
}

func (m *Model) handleRedo() {
	if len(m.redoStack) == 0 {
		m.statusMsg = "nothing to redo"
		return
	}
	rec := m.redoStack[len(m.redoStack)-1]
	m.redoStack = m.redoStack[:len(m.redoStack)-1]
	if rec.isDelete {
		m.redoDelete(rec)
		return
	}
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

// redoDelete re-applies a :delete that was undone (#1). A generic Save
// (what handleRedo does for every other action) would recreate the note
// instead of re-trashing it — newNote == oldNote for a delete record (see
// undoRecord.isDelete), so there's no "new content" to save, only "gone
// again". Mirrors cmdDelete's exact side effects rather than duplicating a
// divergent path.
func (m *Model) redoDelete(rec undoRecord) {
	next := rec.newNote
	if next.State == vault.StateProjects && next.Project != "" {
		m.recordDetach(&next)
	}
	if err := m.vault.Trash(&next); err != nil {
		m.statusMsg = "redo error: " + err.Error()
		return
	}
	m.index.Delete(next.ID)
	delete(m.titleSet, strings.ToLower(next.Title))
	for i := range m.splits {
		if m.splits[i].viewer.note != nil && m.splits[i].viewer.note.ID == next.ID {
			m.splits[i].activeView = viewList
			m.splits[i].viewer = newViewer()
			m.splits[i].history = nil
			m.splits[i].histIdx = -1
		}
	}
	refreshCounts(m)
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
