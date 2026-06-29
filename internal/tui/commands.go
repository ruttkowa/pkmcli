package tui

import (
	"fmt"
	"strings"
	"time"

	"pkm/internal/vault"

	tea "github.com/charmbracelet/bubbletea"
)

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
		}
	}

	switch cmd {
	case "new":
		return m.cmdNew(args)
	case "insert":
		return m.cmdInsert(args)
	case "open":
		return m.cmdOpen(args)
	case "move":
		return m.cmdMove(raw)
	case "archive":
		return m.cmdArchive(args)
	case "split":
		return m.cmdSplit(args)
	case "close":
		return m.cmdClose()
	case "theme":
		return m.cmdTheme(args)
	case "config":
		return m.cmdConfig(args)
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
			m.palette = newPaletteWithInput("add project ", sortedNotes(m.vault), m.vault.Projects.ActiveNames())
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
	if args[0] == "theme" {
		return m.cmdTheme(args[1:])
	}
	return fmt.Sprintf("unknown config key: %q", args[0]), nil
}

// cmdInsert inserts a template note's body into the currently open note.
func (m *Model) cmdInsert(args []string) (string, tea.Cmd) {
	if len(args) == 0 {
		return "usage: :insert <template>", nil
	}
	sp := &m.splits[m.activeSplit]
	if sp.viewer.note == nil {
		return "no note open", nil
	}
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
	n := sp.viewer.note
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

	// Ensure the project exists (creates it if new, errors if max reached).
	if _, err := m.vault.Projects.EnsureProject(projectName); err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}

	// Detach from previous project if switching.
	if n.State == vault.StateProjects && n.Project != "" && n.Project != projectName {
		m.recordDetach(n)
	}

	n.Project = projectName

	// Always move to projects state when assigning a project.
	if n.State != vault.StateProjects {
		if err := m.vault.SetState(n, vault.StateProjects); err != nil {
			return fmt.Sprintf("error: %v", err), nil
		}
	} else {
		if err := m.vault.Save(n); err != nil {
			return fmt.Sprintf("error saving: %v", err), nil
		}
	}

	m.recordAttach(n)
	m.index.Upsert(n)
	return refreshCounts(m), nil
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
	m.sidebar = m.sidebar.refreshNotes()
	return ""
}
