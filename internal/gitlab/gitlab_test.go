package gitlab

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientIssuesPaginationAuthAndEscapedProject(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.EscapedPath()+"?"+r.URL.RawQuery)
		if r.Header.Get("PRIVATE-TOKEN") != "secret" {
			t.Errorf("PRIVATE-TOKEN = %q", r.Header.Get("PRIVATE-TOKEN"))
		}
		if r.URL.Query().Get("page") == "1" {
			w.Header().Set("X-Next-Page", "2")
			w.Write([]byte(`[{"iid":1,"title":"one"}]`))
			return
		}
		w.Write([]byte(`[{"iid":2,"title":"two"}]`))
	}))
	defer server.Close()

	issues, err := NewClient(server.URL, "secret").ListIssues("group/repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 2 || issues[1].IID != 2 {
		t.Fatalf("issues = %#v", issues)
	}
	if len(paths) != 2 || !strings.Contains(paths[0], "group%2Frepo") {
		t.Fatalf("paths = %v", paths)
	}
}

func TestClientNon2xxDoesNotLeakToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	_, err := NewClient(server.URL, "do-not-leak").ListIssues("group/repo")
	if err == nil || err.Error() != "gitlab: group/repo: HTTP 401" {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(err.Error(), "do-not-leak") {
		t.Fatal("token leaked")
	}
}

func TestClientFiltersSystemComments(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"body":"keep","system":false},{"body":"drop","system":true}]`))
	}))
	defer server.Close()
	comments, err := NewClient(server.URL, "token").ListComments("group/repo", 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 1 || comments[0].Body != "keep" {
		t.Fatalf("comments = %#v", comments)
	}
}
