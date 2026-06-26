package tui

import (
	"fmt"
	"strings"

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

	switch cmd {
	case "new":
		return m.cmdNew(args)
	case "open":
		return m.cmdOpen(args)
	case "search":
		return m.cmdSearch(args)
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
	m.splits[m.activeSplit].openNote(n)
	l := m.computeLayout()
	m.splits[m.activeSplit].viewer = m.splits[m.activeSplit].viewer.preRender(l.paneWidth)
	return refreshCounts(m), nil
}

func (m *Model) cmdOpen(args []string) (string, tea.Cmd) {
	if len(args) == 0 {
		return "usage: :open <note>", nil
	}
	query := strings.Join(args, " ")
	n, err := m.vault.FindByTitle(query)
	if err != nil {
		return fmt.Sprintf("not found: %q", query), nil
	}
	m.splits[m.activeSplit].openNote(n)
	l := m.computeLayout()
	m.splits[m.activeSplit].viewer = m.splits[m.activeSplit].viewer.preRender(l.paneWidth)
	return "opened: " + n.Title, nil
}

func (m *Model) cmdSearch(args []string) (string, tea.Cmd) {
	if len(args) == 0 {
		return "usage: :search <query>  or  :search #tag", nil
	}
	query := strings.Join(args, " ")

	var ids []string
	var err error
	if strings.HasPrefix(query, "#") {
		ids, err = m.index.SearchByTag(strings.TrimPrefix(query, "#"))
	} else {
		ids, err = m.index.Search(query)
	}
	if err != nil {
		return fmt.Sprintf("search error: %v", err), nil
	}
	if len(ids) == 0 {
		return fmt.Sprintf("no results for %q", query), nil
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

	if target == vault.StateProjects {
		counts, _ := m.index.CountByState()
		if counts[vault.StateProjects] >= 4 {
			return "max 4 active projects reached", nil
		}
	}

	if err := m.vault.SetState(n, target); err != nil {
		return fmt.Sprintf("error: %v", err), nil
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
		m.splits[m.activeSplit].viewer = m.splits[m.activeSplit].viewer.preRender(l.paneWidth)
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
		return "usage: :theme dark|light", nil
	}
	switch strings.ToLower(args[0]) {
	case "dark":
		activeTheme = DarkTheme
	case "light":
		activeTheme = LightTheme
	default:
		return fmt.Sprintf("unknown theme %q (use: dark, light)", args[0]), nil
	}
	// Bust viewer caches so glamour re-renders with the new style.
	for i := range m.splits {
		m.splits[i].viewer.rendered = ""
		m.splits[i].viewer.renderWidth = 0
	}
	return "theme: " + activeTheme.Name, nil
}

func refreshCounts(m *Model) string {
	counts, _ := m.index.CountByState()
	m.sidebar = m.sidebar.withCounts(counts)
	m.sidebar = m.sidebar.refreshNotes()
	return ""
}
