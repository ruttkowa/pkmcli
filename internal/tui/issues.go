package tui

import (
	"fmt"
	"strings"
	"time"

	"pkm/internal/gitlab"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type issuesFetchedMsg struct {
	projects  map[string][]gitlab.Issue
	errs      map[string]error
	fetchedAt time.Time
}

type issueCommentsMsg struct {
	project  string
	iid      int
	comments []gitlab.Comment
	err      error
}

func fetchIssuesCmd(client *gitlab.Client, projects []string) tea.Cmd {
	return func() tea.Msg {
		msg := issuesFetchedMsg{
			projects:  map[string][]gitlab.Issue{},
			errs:      map[string]error{},
			fetchedAt: time.Now(),
		}
		for _, project := range projects {
			issues, err := client.ListIssues(project)
			if err != nil {
				msg.errs[project] = err
			} else {
				msg.projects[project] = issues
			}
		}
		return msg
	}
}

func fetchCommentsCmd(client *gitlab.Client, project string, iid int) tea.Cmd {
	return func() tea.Msg {
		comments, err := client.ListComments(project, iid)
		return issueCommentsMsg{project: project, iid: iid, comments: comments, err: err}
	}
}

func buildIssueRows(cfg AppConfig, cache gitlab.Cache, token string) []taskOverviewRow {
	if len(cfg.GitLabProjects) == 0 {
		return nil
	}
	var rows []taskOverviewRow
	for _, project := range cfg.GitLabProjects {
		rows = append(rows, taskOverviewRow{})
		header := project + " — "
		if token == "" {
			header += "set PKM_GITLAB_TOKEN to sync"
		} else {
			header += "synced " + relativeSyncAge(cache.FetchedAt)
		}
		rows = append(rows, taskOverviewRow{projectHeader: header, issueProject: project})
		for i := range cache.Projects[project] {
			issue := cache.Projects[project][i]
			rows = append(rows, taskOverviewRow{issue: &issue, issueProject: project})
		}
	}
	return rows
}

func relativeSyncAge(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

func (d *issueDetailPane) markdown() string {
	if d.issue == nil {
		return ""
	}
	labels := strings.Join(d.issue.Labels, ", ")
	meta := fmt.Sprintf("#%d · %s · %s", d.issue.IID, d.issue.Author.Name, d.issue.CreatedAt.Format("2006-01-02"))
	if labels != "" {
		meta += " · " + labels
	}
	description := d.issue.Description
	if strings.TrimSpace(description) == "" {
		description = "_no description_"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n%s\n%s\n\n%s\n\n---\n## Comments (%d)\n",
		d.issue.Title, meta, d.issue.WebURL, description, d.issue.UserNotesCount)
	switch {
	case d.loading:
		b.WriteString("syncing comments…\n")
	case d.commentsErr != "":
		b.WriteString("comments unavailable (offline?)\n")
	default:
		for _, comment := range d.comments {
			fmt.Fprintf(&b, "\n**%s** · %s\n%s\n", comment.Author.Name, comment.CreatedAt.Format("2006-01-02"), comment.Body)
		}
	}
	return b.String()
}

func (d *issueDetailPane) render(width, height int) string {
	if d.rendered == "" || d.renderWidth != width {
		d.rendered = renderMarkdownDocument(d.markdown(), width)
		d.renderWidth = width
	}
	lines := strings.Split(strings.TrimRight(d.rendered, "\n"), "\n")
	maxOff := max(0, len(lines)-height)
	d.scrollOff = min(max(0, d.scrollOff), maxOff)
	end := min(len(lines), d.scrollOff+height)
	visible := lines[d.scrollOff:end]
	for len(visible) < height {
		visible = append(visible, "")
	}
	return lipgloss.NewStyle().Width(width).Height(height).Background(activeTheme.Bg).Render(strings.Join(visible, "\n"))
}
