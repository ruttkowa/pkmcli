package vault

import "time"

type NoteState string

const (
	StateInbox    NoteState = "inbox"
	StateProjects NoteState = "projects"
	StateAreas    NoteState = "areas"
	StateResearch NoteState = "research"
	StateArchive  NoteState = "archive"
)

var AllStates = []NoteState{
	StateInbox,
	StateProjects,
	StateAreas,
	StateResearch,
	StateArchive,
}

type Note struct {
	// Frontmatter fields
	ID      string    `yaml:"id"`
	Title   string    `yaml:"title"`
	Created time.Time `yaml:"created"`
	Updated time.Time `yaml:"updated"`
	State   NoteState `yaml:"state"`
	Project string    `yaml:"project,omitempty"`
	Tags    []string  `yaml:"tags,omitempty"`

	// Runtime fields (not serialized)
	Path string `yaml:"-"`
	Body string `yaml:"-"`
}

type Vault struct {
	Root     string        // absolute path to vault root
	Projects *ProjectStore // persistent project registry
}
