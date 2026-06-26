package tui

import (
	"fmt"
	"strconv"
	"strings"

	"pkm/internal/index"
	"pkm/internal/vault"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// sidebarItem is one row in the flat navigation list: either a section header
// or a note title under an expanded section.
type sidebarItem struct {
	isSection bool
	state     vault.NoteState
	note      *vault.Note
}

type sidebarModel struct {
	index        *index.Index
	vault        *vault.Vault
	activeState  vault.NoteState
	cursor       int
	counts       map[vault.NoteState]int
	expanded     map[vault.NoteState]bool
	notesByState map[vault.NoteState][]*vault.Note
	selected     bool
	selectedNote *vault.Note // non-nil when a note title was chosen
}

func newSidebar(idx *index.Index, v *vault.Vault) sidebarModel {
	return sidebarModel{
		index:        idx,
		vault:        v,
		activeState:  vault.StateInbox,
		counts:       map[vault.NoteState]int{},
		expanded:     map[vault.NoteState]bool{},
		notesByState: map[vault.NoteState][]*vault.Note{},
	}
}

func (s sidebarModel) init() tea.Cmd {
	return func() tea.Msg {
		counts, _ := s.index.CountByState()
		return countsRefreshedMsg{counts: counts}
	}
}

func (s sidebarModel) withCounts(counts map[vault.NoteState]int) sidebarModel {
	s.counts = counts
	return s
}

// refreshNotes reloads note lists for all currently expanded sections.
func (s sidebarModel) refreshNotes() sidebarModel {
	for state, expanded := range s.expanded {
		if expanded {
			notes, _ := s.vault.ListByState(state)
			s.notesByState[state] = notes
		}
	}
	return s
}

// items returns the flat ordered list of all visible sidebar rows.
func (s sidebarModel) items() []sidebarItem {
	var out []sidebarItem
	for _, state := range vault.AllStates {
		out = append(out, sidebarItem{isSection: true, state: state})
		if s.expanded[state] {
			for _, n := range s.notesByState[state] {
				out = append(out, sidebarItem{isSection: false, state: state, note: n})
			}
		}
	}
	return out
}

func (s sidebarModel) update(msg tea.KeyMsg) (sidebarModel, tea.Cmd) {
	items := s.items()

	switch msg.String() {
	case "j", "down":
		if s.cursor < len(items)-1 {
			s.cursor++
		}
	case "k", "up":
		if s.cursor > 0 {
			s.cursor--
		}
	case "left":
		if len(items) == 0 {
			break
		}
		item := items[s.cursor]
		if item.isSection && s.expanded[item.state] {
			s.expanded[item.state] = false
			newItems := s.items()
			for i, it := range newItems {
				if it.isSection && it.state == item.state {
					s.cursor = i
					break
				}
			}
			s.activeState = item.state
			s.selected = true
			s.selectedNote = nil
		} else if !item.isSection {
			// On a note: move cursor up to the section header.
			for i := s.cursor - 1; i >= 0; i-- {
				if items[i].isSection {
					s.cursor = i
					break
				}
			}
		}
	case "right":
		if len(items) == 0 {
			break
		}
		item := items[s.cursor]
		if item.isSection && !s.expanded[item.state] {
			s.expanded[item.state] = true
			notes, _ := s.vault.ListByState(item.state)
			s.notesByState[item.state] = notes
			s.activeState = item.state
			s.selected = true
			s.selectedNote = nil
		}
	case "enter":
		if len(items) == 0 {
			break
		}
		item := items[s.cursor]
		if item.isSection {
			if s.expanded[item.state] {
				// Collapse: move cursor back to the section header.
				s.expanded[item.state] = false
				newItems := s.items()
				for i, it := range newItems {
					if it.isSection && it.state == item.state {
						s.cursor = i
						break
					}
				}
			} else {
				// Expand and load notes for this section.
				s.expanded[item.state] = true
				notes, _ := s.vault.ListByState(item.state)
				s.notesByState[item.state] = notes
			}
			s.activeState = item.state
			s.selected = true
			s.selectedNote = nil
		} else {
			// Note title selected: open in main pane.
			s.selectedNote = item.note
			s.selected = true
		}
	}
	return s, nil
}

func (s sidebarModel) render(width, height int, focused bool) string {
	t := activeTheme
	accentColor := t.BorderNormal
	if focused {
		accentColor = t.Accent
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(accentColor).Padding(0, 1)

	// All styles set outer width; Padding(0,1) reserves 2 chars for padding.
	activeStyle := lipgloss.NewStyle().
		Background(t.Accent).Foreground(t.AccentFg).
		Width(width).Padding(0, 1)
	cursorStyle := lipgloss.NewStyle().
		Background(t.Bg).Foreground(t.Cursor).Width(width).Padding(0, 1)
	dimCursorStyle := lipgloss.NewStyle().
		Background(t.Bg).Foreground(t.TextMuted).Width(width).Padding(0, 1)
	normalStyle := lipgloss.NewStyle().
		Background(t.Bg).Foreground(t.TextPrimary).Width(width).Padding(0, 1)

	noteStyle := lipgloss.NewStyle().
		Background(t.Bg).Foreground(t.TextMuted).Width(width).Padding(0, 1)
	cursorNoteStyle := lipgloss.NewStyle().
		Background(t.Bg).Foreground(t.Cursor).Width(width).Padding(0, 1)
	dimNoteStyle := lipgloss.NewStyle().
		Background(t.Bg).Foreground(t.TextDim).Width(width).Padding(0, 1)

	items := s.items()

	var b strings.Builder
	b.WriteString(titleStyle.Render("SECTIONS") + "\n\n")

	// inner = content area width (outer width minus left+right padding).
	inner := width - 2
	if inner < 4 {
		inner = 4
	}

	for i, item := range items {
		isCursor := i == s.cursor
		var line string

		if item.isSection {
			exp := "▶ "
			if s.expanded[item.state] {
				exp = "▼ "
			}
			countStr := strconv.Itoa(s.counts[item.state])
			name := capitalize(string(item.state))
			// "▶ " is 2 display chars; leave 1 space + countStr at right.
			nameW := inner - 2 - 1 - len(countStr)
			if nameW < 1 {
				nameW = 1
			}
			content := fmt.Sprintf("%s%-*s %s", exp, nameW, name, countStr)

			isActive := item.state == s.activeState
			switch {
			case isActive && isCursor && focused:
				line = activeStyle.Render(content)
			case isActive:
				bullet := fmt.Sprintf("● %-*s %s", nameW, name, countStr)
				line = activeStyle.Render(bullet)
			case isCursor && focused:
				line = cursorStyle.Render(content)
			case isCursor:
				line = dimCursorStyle.Render(content)
			default:
				line = normalStyle.Render(content)
			}
		} else {
			const indent = "  · " // 4 display chars
			titleW := inner - len([]rune(indent))
			if titleW < 1 {
				titleW = 1
			}
			content := indent + truncate(item.note.Title, titleW)

			switch {
			case isCursor && focused:
				line = cursorNoteStyle.Render(content)
			case isCursor:
				line = dimNoteStyle.Render(content)
			default:
				line = noteStyle.Render(content)
			}
		}
		b.WriteString(line + "\n")
	}

	return lipgloss.NewStyle().
		Width(width).Height(height).
		Background(activeTheme.Bg).
		Render(b.String())
}
