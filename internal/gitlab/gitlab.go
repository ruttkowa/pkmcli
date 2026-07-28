package gitlab

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Issue struct {
	IID         int       `json:"iid"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Labels      []string  `json:"labels"`
	WebURL      string    `json:"web_url"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Author      struct {
		Name string `json:"name"`
	} `json:"author"`
	UserNotesCount int `json:"user_notes_count"`
}

type Comment struct {
	Body      string    `json:"body"`
	System    bool      `json:"system"`
	CreatedAt time.Time `json:"created_at"`
	Author    struct {
		Name string `json:"name"`
	} `json:"author"`
}

type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

func NewClient(baseURL, token string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Token:   token,
		HTTP:    &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Client) ListIssues(project string) ([]Issue, error) {
	var issues []Issue
	page := "1"
	for count := 0; count < 3; count++ {
		endpoint := fmt.Sprintf("%s/api/v4/projects/%s/issues?state=opened&per_page=100&page=%s",
			c.BaseURL, url.PathEscape(project), url.QueryEscape(page))
		var batch []Issue
		next, err := c.get(project, endpoint, &batch)
		if err != nil {
			return nil, err
		}
		issues = append(issues, batch...)
		if next == "" {
			break
		}
		page = next
	}
	return issues, nil
}

func (c *Client) ListComments(project string, iid int) ([]Comment, error) {
	endpoint := fmt.Sprintf("%s/api/v4/projects/%s/issues/%s/notes?sort=asc&per_page=100",
		c.BaseURL, url.PathEscape(project), strconv.Itoa(iid))
	var raw []Comment
	if _, err := c.get(project, endpoint, &raw); err != nil {
		return nil, err
	}
	comments := make([]Comment, 0, len(raw))
	for _, comment := range raw {
		if !comment.System {
			comments = append(comments, comment)
		}
	}
	return comments, nil
}

func (c *Client) get(project, endpoint string, target any) (string, error) {
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("PRIVATE-TOKEN", c.Token)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("gitlab: %s: %w", project, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("gitlab: %s: HTTP %d", project, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return "", fmt.Errorf("gitlab: %s: %w", project, err)
	}
	return resp.Header.Get("X-Next-Page"), nil
}
