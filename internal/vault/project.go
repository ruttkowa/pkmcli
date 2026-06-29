package vault

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	projectsFilename = "projects.yaml"
	MaxProjects      = 4
)

type HistoryKind string

const (
	HistoryKindAttached HistoryKind = "attached"
	HistoryKindDetached HistoryKind = "detached"
	HistoryKindNote     HistoryKind = "note" // Hemingway bridge user entry
)

type HistoryEntry struct {
	Timestamp time.Time   `yaml:"timestamp"`
	Kind      HistoryKind `yaml:"kind"`
	NoteID    string      `yaml:"note_id,omitempty"`
	NoteTitle string      `yaml:"note_title,omitempty"`
	Message   string      `yaml:"message,omitempty"`
}

type Project struct {
	Name     string         `yaml:"name"`
	Created  time.Time      `yaml:"created"`
	Archived bool           `yaml:"archived,omitempty"`
	History  []HistoryEntry `yaml:"history,omitempty"`
}

type projectsData struct {
	Projects []*Project `yaml:"projects"`
}

// ProjectStore manages project entities in vault/.pkm/projects.yaml.
type ProjectStore struct {
	path     string
	projects []*Project
}

func openProjectStore(vaultRoot string) (*ProjectStore, error) {
	path := filepath.Join(vaultRoot, ".pkm", projectsFilename)
	ps := &ProjectStore{path: path}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return ps, nil
	}
	if err != nil {
		return nil, err
	}

	var pd projectsData
	if err := yaml.Unmarshal(data, &pd); err != nil {
		return nil, fmt.Errorf("projects.yaml: %w", err)
	}
	ps.projects = pd.Projects
	return ps, nil
}

func (ps *ProjectStore) save() error {
	pd := projectsData{Projects: ps.projects}
	data, err := yaml.Marshal(&pd)
	if err != nil {
		return err
	}
	return os.WriteFile(ps.path, data, 0o644)
}

// ListActive returns all non-archived projects.
func (ps *ProjectStore) ListActive() []*Project {
	var out []*Project
	for _, p := range ps.projects {
		if !p.Archived {
			out = append(out, p)
		}
	}
	return out
}

// Get returns the project with the given name, regardless of archived state.
func (ps *ProjectStore) Get(name string) (*Project, bool) {
	for _, p := range ps.projects {
		if p.Name == name {
			return p, true
		}
	}
	return nil, false
}

// Create adds a new active project. Returns error if MaxProjects is reached or name exists.
func (ps *ProjectStore) Create(name string) (*Project, error) {
	if _, exists := ps.Get(name); exists {
		return nil, fmt.Errorf("project %q already exists", name)
	}
	if len(ps.ListActive()) >= MaxProjects {
		return nil, fmt.Errorf("max %d active projects reached — archive a project first", MaxProjects)
	}
	p := &Project{
		Name:    name,
		Created: time.Now().Truncate(time.Second),
	}
	ps.projects = append(ps.projects, p)
	return p, ps.save()
}

// EnsureProject returns an existing project or creates it if the name is new.
func (ps *ProjectStore) EnsureProject(name string) (*Project, error) {
	if p, ok := ps.Get(name); ok {
		return p, nil
	}
	return ps.Create(name)
}

// AddHistory appends an entry to the named project and saves.
func (ps *ProjectStore) AddHistory(name string, entry HistoryEntry) error {
	p, ok := ps.Get(name)
	if !ok {
		return fmt.Errorf("project %q not found", name)
	}
	p.History = append(p.History, entry)
	return ps.save()
}

// Archive marks a project as archived and saves.
func (ps *ProjectStore) Archive(name string) error {
	p, ok := ps.Get(name)
	if !ok {
		return fmt.Errorf("project %q not found", name)
	}
	p.Archived = true
	return ps.save()
}

// ActiveNames returns the names of all active projects in creation order.
func (ps *ProjectStore) ActiveNames() []string {
	active := ps.ListActive()
	names := make([]string, len(active))
	for i, p := range active {
		names[i] = p.Name
	}
	return names
}
