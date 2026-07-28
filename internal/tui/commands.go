package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"pkm/internal/vault"

	tea "github.com/charmbracelet/bubbletea"
)

// isLiveEditCommand reports whether raw's verb operates on an open editor's
// live buffer (cmdInsert) rather than the saved note, so it's safe to run
// without first committing an in-progress draft.
func isLiveEditCommand(raw string) bool {
	verb, _, _ := strings.Cut(strings.TrimPrefix(raw, ":"), " ")
	return verb == "insert"
}

// handleCommand parses and executes a palette command, returning a status message and optional Cmd.
func (m *Model) handleCommand(raw string) (string, tea.Cmd) {
	raw = strings.TrimPrefix(raw, ":")
	parts := strings.Fields(raw)
	if len(parts) == 0 {
		return "", nil
	}

	cmd, args := parts[0], parts[1:]

	// Check compound commands (two-word verbs) before single-word switch.
	if len(parts) >= 2 {
		switch parts[0] + " " + parts[1] {
		case "add project":
			return m.cmdProject(parts[2:])
		case "new project":
			return m.cmdNewProject(parts[2:])
		case "new template":
			return m.cmdNewTemplate(parts[2:])
		case "insert var":
			return m.cmdInsertVar(parts[2:])
		}
	}

	switch cmd {
	case "new":
		return m.cmdNew(args)
	case "insert":
		return m.cmdInsert(args)
	case "open":
		return m.cmdOpen(args)
	case "search":
		return m.cmdSearch(args)
	case "move":
		return m.cmdMove(raw)
	case "archive":
		return m.cmdArchive(args)
	case "delete":
		return m.cmdDelete(args)
	case "split":
		return m.cmdSplit(args)
	case "close":
		return m.cmdClose()
	case "theme":
		return m.cmdTheme(args)
	case "config":
		return m.cmdConfig(args)
	case "import":
		return m.cmdImport(args)
	case "export":
		return m.cmdExport(args)
	case "tasks":
		return m.cmdTasks()
	case "trash":
		return m.cmdTrash()
	case "help":
		return m.cmdHelp()
	case "quit", "exit", "q":
		return m.cmdQuit()
	default:
		return fmt.Sprintf("unknown command: %q", cmd), nil
	}
}

func (m *Model) cmdNew(args []string) (string, tea.Cmd) {
	title := strings.Join(args, " ")
	title = strings.Trim(title, `"'`)
	if title == "" {
		return "usage: :new \"Title\"", nil
	}
	n, err := m.vault.Create(title)
	if err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}
	m.index.Upsert(n)
	m.titleSet[strings.ToLower(n.Title)] = true
	m.splits[m.activeSplit].openNote(n)
	l := m.computeLayout()
	m.splits[m.activeSplit].viewer = m.splits[m.activeSplit].viewer.preRender(l.paneWidth, m.titleSet)
	return refreshCounts(m), nil
}

func (m *Model) cmdOpen(args []string) (string, tea.Cmd) {
	if len(args) == 0 {
		return "usage: :open <note · query · #tag>", nil
	}
	query := strings.Join(args, " ")

	// Tag search — show matching notes as a list.
	if strings.HasPrefix(query, "#") {
		ids, err := m.index.SearchByTag(strings.TrimPrefix(query, "#"))
		if err != nil {
			return fmt.Sprintf("search error: %v", err), nil
		}
		return m.showSearchResults(ids, query)
	}

	// Try title match first — open directly if found.
	if n, err := m.vault.FindByTitle(query); err == nil {
		m.splits[m.activeSplit].openNote(n)
		l := m.computeLayout()
		m.splits[m.activeSplit].viewer = m.splits[m.activeSplit].viewer.preRender(l.paneWidth, m.titleSet)
		return "opened: " + n.Title, nil
	}

	// Fall back to full-text search — show results as a list.
	ids, err := m.index.Search(query)
	if err != nil {
		return fmt.Sprintf("search error: %v", err), nil
	}
	return m.showSearchResults(ids, query)
}

func (m *Model) showSearchResults(ids []string, query string) (string, tea.Cmd) {
	if len(ids) == 0 {
		return fmt.Sprintf("not found: %q", query), nil
	}
	all, _ := m.vault.ListAll()
	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}
	var results []*vault.Note
	for _, n := range all {
		if idSet[n.ID] {
			results = append(results, n)
		}
	}
	m.noteList = m.noteList.withNotes(results)
	m.searchResults = results
	m.splits[m.activeSplit].activeView = viewList
	return fmt.Sprintf("%d result(s) for %q", len(results), query), nil
}

// cmdSearch fuzzy-searches titles and content across the whole vault and
// shows every hit as a list (reusing the note list view, exactly like
// cmdOpen's full-text fallback) — the "no explicit pick" path from the
// palette's live dropdown. m.searchResults marks the list so opening a note
// from it remembers to return here on "back" (see splitPane.searchReturn).
func (m *Model) cmdSearch(args []string) (string, tea.Cmd) {
	if len(args) == 0 {
		return "usage: :search <query>", nil
	}
	query := strings.Join(args, " ")
	all, _ := m.vault.ListAll()
	results := fuzzySearchNotes(query, all)
	if len(results) == 0 {
		return fmt.Sprintf("not found: %q", query), nil
	}
	m.noteList = m.noteList.withNotes(results)
	m.searchResults = results
	m.splits[m.activeSplit].activeView = viewList
	return fmt.Sprintf("%d result(s) for %q", len(results), query), nil
}

func (m *Model) cmdMove(raw string) (string, tea.Cmd) {
	arrow := "→"
	if !strings.Contains(raw, arrow) {
		arrow = "->"
	}
	before, after, found := strings.Cut(strings.TrimPrefix(raw, "move "), arrow)
	if !found {
		return "usage: :move <note> → <state>", nil
	}
	noteQuery := strings.TrimSpace(before)
	stateStr := strings.TrimSpace(after)

	n, err := m.vault.FindByTitle(noteQuery)
	if err != nil {
		return fmt.Sprintf("not found: %q", noteQuery), nil
	}

	var target vault.NoteState
	switch strings.ToLower(stateStr) {
	case "inbox":
		target = vault.StateInbox
	case "projects", "project":
		target = vault.StateProjects
	case "areas", "area":
		target = vault.StateAreas
	case "research":
		target = vault.StateResearch
	case "archive":
		target = vault.StateArchive
	default:
		return fmt.Sprintf("unknown state: %q", stateStr), nil
	}

	// Detach from project when leaving projects state.
	if n.State == vault.StateProjects && n.Project != "" && target != vault.StateProjects {
		m.recordDetach(n)
	}

	if target == vault.StateProjects {
		if n.Project == "" {
			// No project assigned: open the project picker. Store the pending note
			// so cmdProject can complete the move after the user picks.
			m.pendingMoveNote = n
			m.showPalette = true
			m.palette = newPaletteWithInput("add project ", sortedNotes(m.vault), m.vault.Projects.ActiveNames()).withVariables(m.variableNames())
			return "choose a project to assign this note to", nil
		}
		if _, err := m.vault.Projects.EnsureProject(n.Project); err != nil {
			return fmt.Sprintf("error: %v", err), nil
		}
	}

	if err := m.vault.SetState(n, target); err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}

	// Attach to project when entering projects state.
	if target == vault.StateProjects && n.Project != "" {
		m.recordAttach(n)
	}

	m.index.Upsert(n)
	return refreshCounts(m), nil
}

func (m *Model) cmdArchive(args []string) (string, tea.Cmd) {
	if len(args) == 0 {
		return "usage: :archive <note>", nil
	}
	query := strings.Join(args, " ")
	n, err := m.vault.FindByTitle(query)
	if err != nil {
		return fmt.Sprintf("not found: %q", query), nil
	}
	if n.State == vault.StateProjects && n.Project != "" {
		m.recordDetach(n)
	}
	if err := m.vault.SetState(n, vault.StateArchive); err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}
	m.index.Upsert(n)
	return refreshCounts(m), nil
}

// cmdDelete moves a note's file into the vault's trash (#1) instead of
// permanently removing it — recoverable via :trash within the configured
// retention window. The note is also snapshotted to the undo stack (Save
// recreates it at its original path), so Ctrl+Z right after deleting is the
// immediate safety net; trash is the durable one.
func (m *Model) cmdDelete(args []string) (string, tea.Cmd) {
	if len(args) == 0 {
		return "usage: :delete <note>", nil
	}
	query := strings.Join(args, " ")
	n, err := m.vault.FindByTitle(query)
	if err != nil {
		return fmt.Sprintf("not found: %q", query), nil
	}
	if n.State == vault.StateProjects && n.Project != "" {
		m.recordDetach(n)
	}
	oldNote := *n
	if err := m.vault.Trash(n); err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}
	m.index.Delete(n.ID)
	delete(m.titleSet, strings.ToLower(n.Title))

	m.undoStack = append(m.undoStack, undoRecord{oldNote: oldNote, newNote: oldNote, isDelete: true})
	if len(m.undoStack) > 20 {
		m.undoStack = m.undoStack[1:]
	}
	m.redoStack = nil

	// Any split showing the deleted note falls back to the list view.
	for i := range m.splits {
		if m.splits[i].viewer.note != nil && m.splits[i].viewer.note.ID == n.ID {
			m.splits[i].activeView = viewList
			m.splits[i].viewer = newViewer()
			m.splits[i].history = nil
			m.splits[i].histIdx = -1
		}
	}

	refreshCounts(m)
	return "deleted: " + n.Title, nil
}

func (m *Model) cmdSplit(args []string) (string, tea.Cmd) {
	sp := newSplitPane()
	if len(args) > 0 {
		query := strings.Join(args, " ")
		n, err := m.vault.FindByTitle(query)
		if err != nil {
			return fmt.Sprintf("not found: %q", query), nil
		}
		sp.openNote(n)
	}
	m.splits = append(m.splits, sp)
	m.activeSplit = len(m.splits) - 1
	if m.splits[m.activeSplit].viewer.note != nil {
		l := m.computeLayout()
		m.splits[m.activeSplit].viewer = m.splits[m.activeSplit].viewer.preRender(l.paneWidth, m.titleSet)
	}
	return fmt.Sprintf("pane %d opened", m.activeSplit+1), nil
}

func (m *Model) cmdClose() (string, tea.Cmd) {
	if len(m.splits) <= 1 {
		return "cannot close the last pane", nil
	}
	m.splits = append(m.splits[:m.activeSplit], m.splits[m.activeSplit+1:]...)
	if m.activeSplit >= len(m.splits) {
		m.activeSplit = len(m.splits) - 1
	}
	return fmt.Sprintf("%d pane(s) open", len(m.splits)), nil
}

func (m *Model) cmdTheme(args []string) (string, tea.Cmd) {
	if len(args) == 0 {
		names := make([]string, len(ThemeChoices))
		for i, t := range ThemeChoices {
			names[i] = t.Name
		}
		return "usage: :theme " + strings.Join(names, "|"), nil
	}
	name := strings.ToLower(args[0])
	for _, t := range ThemeChoices {
		if t.Name == name {
			activeTheme = t
			m.cfg.Theme = name
			m.bustViewerCaches()
			saveConfig(m.vault, m.cfg)
			return "theme: " + name, nil
		}
	}
	return fmt.Sprintf("unknown theme %q", args[0]), nil
}

func (m *Model) cmdNewProject(args []string) (string, tea.Cmd) {
	if len(args) == 0 {
		return `usage: :new project <name>`, nil
	}
	name := strings.TrimSpace(strings.Join(args, " "))
	name = strings.Trim(name, `"'`)
	if name == "" {
		return `usage: :new project <name>`, nil
	}
	if _, err := m.vault.Projects.Create(name); err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}
	return "project created: " + name, nil
}

func (m *Model) cmdConfig(args []string) (string, tea.Cmd) {
	if len(args) == 0 {
		m.showConfig = true
		m.configView = newConfigPane(m.cfg)
		return "", nil
	}
	switch args[0] {
	case "theme":
		return m.cmdTheme(args[1:])
	case "export":
		return m.cmdConfigExport(args[1:])
	case "import":
		return m.cmdConfigImport(args[1:])
	}
	return fmt.Sprintf("unknown config key: %q", args[0]), nil
}

// cmdImport opens the :import popover, pre-filling the path field if one was
// given on the command line (the actual file read/move happens in
// Model.runImport once the user confirms in the popover).
func (m *Model) cmdImport(args []string) (string, tea.Cmd) {
	m.showImport = true
	m.importView = newImportPane()
	if len(args) > 0 {
		path := strings.Join(args, " ")
		m.importView.pathInput.SetValue(path)
		m.importView.pathInput.CursorEnd()
		m.importView.suggestions = pathSuggestions(path)
	}
	return "", nil
}

// runImport executes the import described by m.importView once confirmed,
// closing the popover on success or leaving it open with an error message
// on failure so the user can correct the path and retry.
func (m *Model) runImport() {
	path := strings.TrimSpace(m.importView.pathInput.Value())
	if path == "" {
		m.importView.errMsg = "enter a file path"
		m.importView.confirmed = false
		return
	}
	state := vault.AllStates[m.importView.destIdx]
	n, err := m.vault.Import(path, state, m.importView.move)
	if err != nil {
		m.importView.errMsg = "import error: " + err.Error()
		m.importView.confirmed = false
		return
	}
	m.index.Upsert(n)
	m.titleSet[strings.ToLower(n.Title)] = true
	m.showImport = false
	m.splits[m.activeSplit].openNote(n)
	l := m.computeLayout()
	m.splits[m.activeSplit].viewer = m.splits[m.activeSplit].viewer.preRender(l.paneWidth, m.titleSet)
	m.statusMsg = refreshCounts(m)
}

// cmdExport opens the :export popover for the currently open note, prefilled
// with the note's own filename (the actual file write happens in
// Model.runExport once the user confirms). Unlike :import, export is
// strictly read-only with respect to the vault: it never modifies, moves,
// or reindexes anything — the written file is outside the vault's purview
// even if the destination happens to be inside it.
func (m *Model) cmdExport(args []string) (string, tea.Cmd) {
	sp := &m.splits[m.activeSplit]
	if sp.viewer.note == nil {
		return "no note open to export", nil
	}
	m.showExport = true
	m.exportView = newExportPane(sp.viewer.note)
	if len(args) > 0 {
		path := strings.Join(args, " ")
		m.exportView.pathInput.SetValue(path)
		m.exportView.pathInput.CursorEnd()
		m.exportView.suggestions = pathSuggestions(path)
	}
	return "", nil
}

// runExport writes the currently open note's raw bytes (frontmatter
// included, byte-identical to the vault copy, so it round-trips back in via
// :import) to the path in m.exportView once confirmed. A bare directory
// path exports under the note's own filename. An existing target requires
// one extra confirm (see exportPane.pendingOverwritePath) before it's
// overwritten. Closes the popover on success; leaves it open with an error
// message on failure so the user can correct the path and retry.
func (m *Model) runExport() {
	sp := &m.splits[m.activeSplit]
	n := sp.viewer.note
	if n == nil {
		m.exportView.errMsg = "no note open to export"
		m.exportView.confirmed = false
		return
	}

	raw := strings.TrimSpace(m.exportView.pathInput.Value())
	if raw == "" {
		m.exportView.errMsg = "enter a file path"
		m.exportView.confirmed = false
		return
	}
	target := expandExportPath(raw)

	filename := vault.Filename(n.ID, n.Title)
	if strings.HasSuffix(target, string(os.PathSeparator)) {
		target = filepath.Join(target, filename)
	} else if info, err := os.Stat(target); err == nil && info.IsDir() {
		target = filepath.Join(target, filename)
	}

	dir := filepath.Dir(target)
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		m.exportView.errMsg = "directory does not exist: " + dir
		m.exportView.confirmed = false
		return
	}

	if _, err := os.Stat(target); err == nil && m.exportView.pendingOverwritePath != target {
		m.exportView.pendingOverwritePath = target
		m.exportView.errMsg = "file exists — press Enter again to overwrite"
		m.exportView.confirmed = false
		return
	}

	data, err := os.ReadFile(n.Path)
	if err != nil {
		m.exportView.errMsg = "export error: " + err.Error()
		m.exportView.confirmed = false
		return
	}
	if err := os.WriteFile(target, data, 0o644); err != nil {
		m.exportView.errMsg = "export error: " + err.Error()
		m.exportView.confirmed = false
		return
	}

	m.showExport = false
	m.statusMsg = "exported: " + target
}

func (m *Model) cmdConfigExport(args []string) (string, tea.Cmd) {
	path := strings.Join(args, " ")
	if err := exportConfig(m.vault, m.cfg, path); err != nil {
		return fmt.Sprintf("export error: %v", err), nil
	}
	return "config exported to " + resolveConfigPath(m.vault, path), nil
}

func (m *Model) cmdConfigImport(args []string) (string, tea.Cmd) {
	path := strings.Join(args, " ")
	cfg, err := importConfig(m.vault, path)
	if err != nil {
		return fmt.Sprintf("import error: %v", err), nil
	}
	m.applyConfig(cfg)
	saveConfig(m.vault, m.cfg)
	return "config imported from " + resolveConfigPath(m.vault, path), nil
}

// cmdInsert inserts a template note's body into the currently open note. If
// the note is mid-edit, it writes straight into the live editor buffer
// instead of the saved note — see isLiveEditCommand.
func (m *Model) cmdInsert(args []string) (string, tea.Cmd) {
	if len(args) == 0 {
		return "usage: :insert <template>", nil
	}
	sp := &m.splits[m.activeSplit]
	query := strings.Join(args, " ")
	tmpl, err := m.vault.FindByTitle(query)
	if err != nil {
		return fmt.Sprintf("template not found: %q", query), nil
	}
	isTemplate := false
	for _, tag := range tmpl.Tags {
		if strings.ToLower(tag) == "template" {
			isTemplate = true
			break
		}
	}
	if !isTemplate {
		return fmt.Sprintf("%q is not tagged as a template", tmpl.Title), nil
	}
	templateBody := strings.TrimRight(tmpl.Body, "\n\r \t")
	if templateBody == "" {
		return "template is empty", nil
	}
	now := timeNow()

	if sp.activeView == viewEdit {
		templateBody = m.substituteVariables(templateBody, sp.editor.note, now)
		current := strings.TrimRight(sp.editor.ta.Value(), "\n\r \t")
		next := templateBody
		if current != "" {
			next = current + "\n" + templateBody
		}
		sp.editor.ta.SetValue(next)
		sp.editor.ta, _ = sp.editor.ta.Update(tea.KeyMsg{Type: tea.KeyCtrlEnd})
		sp.editor.wordCount = countWords(next)
		sp.editor = sp.editor.refreshLinkSuggest()
		return "inserted: " + tmpl.Title, nil
	}

	if sp.viewer.note == nil {
		return "no note open", nil
	}
	n := sp.viewer.note
	templateBody = m.substituteVariables(templateBody, n, now)
	currentBody := strings.TrimRight(n.Body, "\n\r \t")
	if currentBody == "" {
		n.Body = templateBody
	} else {
		n.Body = currentBody + "\n" + templateBody
	}
	if err := m.vault.Save(n); err != nil {
		return fmt.Sprintf("error saving: %v", err), nil
	}
	m.index.Upsert(n)
	sp.viewer = sp.viewer.withNote(n)
	l := m.computeLayout()
	sp.viewer = sp.viewer.preRender(l.paneWidth, m.titleSet)
	return "inserted: " + tmpl.Title, nil
}

// substituteVariables replaces {{name}} placeholders in body: the dynamic
// built-ins (id/title/created, from n; updated, from now) plus every
// user-defined variable from config.
func (m *Model) substituteVariables(body string, n *vault.Note, now time.Time) string {
	body = strings.ReplaceAll(body, "{{id}}", n.ID)
	body = strings.ReplaceAll(body, "{{title}}", n.Title)
	body = strings.ReplaceAll(body, "{{created}}", n.Created.Format("2006-01-02T15:04:05"))
	body = strings.ReplaceAll(body, "{{updated}}", now.Format("2006-01-02T15:04:05"))
	for name, value := range m.cfg.Variables {
		body = strings.ReplaceAll(body, "{{"+name+"}}", value)
	}
	return body
}

// cmdInsertVar inserts a configured variable's value at the cursor in the
// live editor buffer. Unlike :insert <template>, this only makes sense
// mid-edit — there's no "append to a read-only note" analog for a single value.
func (m *Model) cmdInsertVar(args []string) (string, tea.Cmd) {
	if len(args) == 0 {
		return "usage: :insert var <variable>", nil
	}
	name := strings.Join(args, " ")
	val, ok := m.cfg.Variables[name]
	if !ok {
		return fmt.Sprintf("unknown variable: %q", name), nil
	}
	sp := &m.splits[m.activeSplit]
	if sp.activeView != viewEdit {
		return "insert var only works while editing (press e first)", nil
	}
	sp.editor.ta.InsertString(val)
	sp.editor.wordCount = countWords(sp.editor.ta.Value())
	sp.editor = sp.editor.refreshLinkSuggest()
	return "inserted variable: " + name, nil
}

// cmdNewTemplate creates a new note pre-tagged as a template.
func (m *Model) cmdNewTemplate(args []string) (string, tea.Cmd) {
	title := strings.Join(args, " ")
	title = strings.Trim(title, `"'`)
	if title == "" {
		return `usage: :new template "Title"`, nil
	}
	n, err := m.vault.Create(title)
	if err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}
	n.Tags = append(n.Tags, "template")
	if err := m.vault.Save(n); err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}
	m.index.Upsert(n)
	m.titleSet[strings.ToLower(n.Title)] = true
	m.splits[m.activeSplit].openNote(n)
	l := m.computeLayout()
	m.splits[m.activeSplit].viewer = m.splits[m.activeSplit].viewer.preRender(l.paneWidth, m.titleSet)
	return "created template: " + n.Title, nil
}

func (m *Model) cmdTasks() (string, tea.Cmd) {
	m.openTasksOverview()
	return "", nil
}

// cmdTrash opens #1's recovery list (Enter restores, d permanently deletes).
func (m *Model) cmdTrash() (string, tea.Cmd) {
	m.openTrashView()
	return "", nil
}

func (m *Model) cmdHelp() (string, tea.Cmd) {
	sp := &m.splits[m.activeSplit]
	sp.activeView = viewHelp
	sp.helpScrollOff = 0
	m.activePane = paneMain
	return "", nil
}

func (m *Model) cmdQuit() (string, tea.Cmd) {
	saveSession(m.vault, m)
	return "", tea.Quit
}

func (m *Model) cmdProject(args []string) (string, tea.Cmd) {
	if len(args) == 0 {
		return "usage: :add project <name>", nil
	}
	projectName := strings.TrimSpace(strings.Join(args, " "))
	if projectName == "" {
		return "usage: :add project <name>", nil
	}

	// Determine which note to assign: pending move note, then focused note.
	n := m.pendingMoveNote
	m.pendingMoveNote = nil
	if n == nil {
		sp := &m.splits[m.activeSplit]
		if sp.viewer.note == nil {
			return "no note open to assign", nil
		}
		n = sp.viewer.note
	}

	if err := m.assignProjectToNote(n, projectName); err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}
	return refreshCounts(m), nil
}

// assignProjectToNote attaches n to the named project: ensures the project
// exists (creating it if new, erroring if the max-active-projects limit is
// reached), records attach/detach history, forces the note into
// vault.StateProjects, persists it, updates the index, and reveals the note
// in the sidebar's project tree. Shared by cmdProject (:add project /
// Shift+P) and the editor's Project field (commitEditorDraft) so both paths
// behave identically.
func (m *Model) assignProjectToNote(n *vault.Note, projectName string) error {
	if _, err := m.vault.Projects.EnsureProject(projectName); err != nil {
		return err
	}

	// Detach from previous project if switching.
	if n.Project != "" && n.Project != projectName {
		m.recordDetach(n)
	}

	n.Project = projectName

	// Always move to projects state when assigning a project.
	if n.State != vault.StateProjects {
		if err := m.vault.SetState(n, vault.StateProjects); err != nil {
			return err
		}
	} else if err := m.vault.Save(n); err != nil {
		return err
	}

	m.recordAttach(n)
	m.index.Upsert(n)
	m.revealNoteInProjectSidebar(projectName, n)
	return nil
}

// revealNoteInProjectSidebar expands the Projects section and the named
// project's folder so a just-assigned note is visible in the sidebar tree,
// and moves the sidebar cursor onto it.
func (m *Model) revealNoteInProjectSidebar(projectName string, n *vault.Note) {
	m.sidebar.expanded[vault.StateProjects] = true
	m.sidebar.expandedProjects[projectName] = true
	m.sidebar = m.sidebar.refreshNotes()
	for i, item := range m.sidebar.items() {
		if item.isProjectNote && item.note != nil && item.note.ID == n.ID {
			m.sidebar.cursor = i
			break
		}
	}
}

func (m *Model) recordAttach(n *vault.Note) {
	_ = m.vault.Projects.AddHistory(n.Project, vault.HistoryEntry{
		Timestamp: time.Now().Truncate(time.Second),
		Kind:      vault.HistoryKindAttached,
		NoteID:    n.ID,
		NoteTitle: n.Title,
	})
}

func (m *Model) recordDetach(n *vault.Note) {
	_ = m.vault.Projects.AddHistory(n.Project, vault.HistoryEntry{
		Timestamp: time.Now().Truncate(time.Second),
		Kind:      vault.HistoryKindDetached,
		NoteID:    n.ID,
		NoteTitle: n.Title,
	})
}

func refreshCounts(m *Model) string {
	counts, _ := m.index.CountByState()
	m.sidebar = m.sidebar.withCounts(counts)
	m.sidebar.refreshNotesPreservingCursor()
	return ""
}
