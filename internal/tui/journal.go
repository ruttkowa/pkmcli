package tui

import (
	"sort"
	"strings"
	"time"

	"pkm/internal/vault"

	"github.com/charmbracelet/lipgloss"
)

const dailyFolder = "Daily"

type dailyOverviewRow struct {
	header string
	note   *vault.Note
}

type dailyOverviewPane struct {
	rows      []dailyOverviewRow
	cursor    int
	scrollOff int
}

func newDailyOverviewPane(notes []*vault.Note) dailyOverviewPane {
	var daily []*vault.Note
	for _, note := range notes {
		if note.State == vault.StateAreas && note.Folder == dailyFolder {
			daily = append(daily, note)
		}
	}
	sort.Slice(daily, func(i, j int) bool {
		return journalDate(daily[i]).After(journalDate(daily[j]))
	})
	var rows []dailyOverviewRow
	lastYear, lastMonth := -1, time.Month(0)
	for _, note := range daily {
		date := journalDate(note)
		if date.Year() != lastYear {
			rows = append(rows, dailyOverviewRow{header: date.Format("2006")})
			lastYear, lastMonth = date.Year(), 0
		}
		if date.Month() != lastMonth {
			rows = append(rows, dailyOverviewRow{header: "  " + date.Format("January")})
			lastMonth = date.Month()
		}
		rows = append(rows, dailyOverviewRow{note: note})
	}
	pane := dailyOverviewPane{rows: rows}
	pane.cursor = pane.nextNote(0, 1)
	return pane
}

func journalDate(note *vault.Note) time.Time {
	if parsed, err := time.ParseInLocation("2006-01-02", note.Title, time.Local); err == nil {
		return parsed
	}
	return note.Created
}

func (p dailyOverviewPane) nextNote(start, direction int) int {
	if len(p.rows) == 0 {
		return 0
	}
	for i := start; i >= 0 && i < len(p.rows); i += direction {
		if p.rows[i].note != nil {
			return i
		}
	}
	return p.cursor
}

func (p *dailyOverviewPane) render(width, height int, focused bool) string {
	if len(p.rows) == 0 {
		return lipgloss.NewStyle().Width(width).Height(height).Background(activeTheme.Bg).
			Render(lipgloss.NewStyle().Foreground(activeTheme.TextDim).Render("No daily notes yet. Run :jrnl to create today's note."))
	}
	lines := make([]string, len(p.rows))
	for i, row := range p.rows {
		switch {
		case row.note != nil:
			content := "    " + row.note.Title
			style := lipgloss.NewStyle().Foreground(activeTheme.TextPrimary)
			if i == p.cursor {
				bg := activeTheme.Accent
				if !focused {
					bg = activeTheme.BlurredBg
				}
				style = style.Background(bg).Foreground(activeTheme.AccentFg).Width(width)
			}
			lines[i] = style.Render(content)
		case strings.HasPrefix(row.header, "  "):
			lines[i] = lipgloss.NewStyle().Foreground(activeTheme.TextSecond).Render(row.header)
		default:
			lines[i] = lipgloss.NewStyle().Bold(true).Foreground(activeTheme.Accent).Render(row.header)
		}
	}
	p.cursor = min(max(0, p.cursor), len(lines)-1)
	if p.cursor < p.scrollOff {
		p.scrollOff = p.cursor
	}
	if p.cursor >= p.scrollOff+height {
		p.scrollOff = p.cursor - height + 1
	}
	end := min(len(lines), p.scrollOff+height)
	visible := lines[p.scrollOff:end]
	for len(visible) < height {
		visible = append(visible, "")
	}
	return lipgloss.NewStyle().Width(width).Height(height).Background(activeTheme.Bg).Render(strings.Join(visible, "\n"))
}
