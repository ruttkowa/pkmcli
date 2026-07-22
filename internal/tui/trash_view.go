package tui

import (
	"fmt"
	"strings"
	"time"

	"pkm/internal/vault"

	"github.com/charmbracelet/lipgloss"
)

// daysRemaining returns how many whole days a trashed note has left before
// PurgeExpired removes it, clamped at 0 (never negative — a note that's
// already past retention but hasn't been purged yet, e.g. because the app
// wasn't restarted since, still reads as "0 days left" rather than a
// confusing negative count).
func daysRemaining(e vault.TrashEntry, retentionDays int) int {
	elapsed := int(time.Since(e.DeletedAt).Hours() / 24)
	remaining := retentionDays - elapsed
	if remaining < 0 {
		remaining = 0
	}
	return remaining
}

// deletedAgo renders a short relative-time string for a trashed note's
// DeletedAt, coarse enough (days, or "today") that it doesn't need to
// update between renders.
func deletedAgo(e vault.TrashEntry) string {
	days := int(time.Since(e.DeletedAt).Hours() / 24)
	switch {
	case days <= 0:
		return "deleted today"
	case days == 1:
		return "deleted 1 day ago"
	default:
		return fmt.Sprintf("deleted %d days ago", days)
	}
}

// renderTrash formats the trash list, highlighting cursorRow the same way
// the task overview does. confirmID (if non-empty) marks the row awaiting a
// second "d" press to confirm permanent deletion.
func renderTrash(rows []vault.TrashEntry, retentionDays, width, height, scrollOff, cursorRow int, confirmID string, focused bool) string {
	t := activeTheme
	textStyle := lipgloss.NewStyle().Foreground(t.TextPrimary)
	dimStyle := lipgloss.NewStyle().Foreground(t.TextDim)
	warnStyle := lipgloss.NewStyle().Foreground(t.Cursor).Bold(true)
	cursorStyle := lipgloss.NewStyle().Background(t.Accent).Foreground(t.AccentFg)

	if len(rows) == 0 {
		empty := dimStyle.Render("Trash is empty.")
		return lipgloss.NewStyle().Width(width).Height(height).Background(t.Bg).Render(empty)
	}

	lines := make([]string, len(rows))
	for i, e := range rows {
		left := e.Title + "  ·  " + deletedAgo(e)
		remaining := daysRemaining(e, retentionDays)
		right := fmt.Sprintf("%d days left", remaining)
		if remaining == 1 {
			right = "1 day left"
		}
		warn := confirmID == e.ID
		if warn {
			right = "press d again to permanently delete"
		}

		pad := width - len([]rune(left)) - len([]rune(right)) - 2
		if pad < 1 {
			pad = 1
		}
		plain := left + strings.Repeat(" ", pad) + right

		var content string
		switch {
		case i == cursorRow:
			content = cursorStyle.Render(plain)
			if !focused {
				content = lipgloss.NewStyle().Background(t.BlurredBg).Foreground(t.TextPrimary).Render(plain)
			}
		case warn:
			content = warnStyle.Render(left) + strings.Repeat(" ", pad) + warnStyle.Render(right)
		default:
			content = textStyle.Render(left) + strings.Repeat(" ", pad) + dimStyle.Render(right)
		}
		lines[i] = content
	}

	if scrollOff > len(lines) {
		scrollOff = len(lines)
	}
	if scrollOff < 0 {
		scrollOff = 0
	}
	end := scrollOff + height
	if end > len(lines) {
		end = len(lines)
	}
	visible := lines[scrollOff:end]

	blank := lipgloss.NewStyle().Width(width).Background(t.Bg).Render("")
	for len(visible) < height {
		visible = append(visible, blank)
	}

	return lipgloss.NewStyle().Width(width).Height(height).Background(t.Bg).Render(strings.Join(visible, "\n"))
}
