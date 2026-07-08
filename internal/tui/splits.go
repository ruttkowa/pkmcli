package tui

import (
	"pkm/internal/vault"
)

// splitPane is one horizontal split in the main area. It owns its own
// navigation history and viewer state.
type splitPane struct {
	activeView view
	viewer     viewerModel
	editor     editPane
	notes      []*vault.Note // list shown in this pane (when activeView == viewList)
	history    []*vault.Note // back/forward stack
	histIdx    int           // current position in history (-1 = empty)

	// project views
	projectDetail projectDetailPane
	allProjects   []*vault.Project // for viewProjectsOverview

	// section landing page
	sectionLanding sectionLandingPane

	helpScrollOff int // scroll position within the help view

	// searchReturn is non-nil when the note currently open (or the one at
	// the root of this split's history) was opened from a search results
	// list — once real history is exhausted, "back" restores this list
	// instead of falling through to whatever view preceded the search.
	// Cleared by any ordinary openNote, so it only applies to the note(s)
	// reached directly from that particular search.
	searchReturn []*vault.Note
}

func newSplitPane() splitPane {
	return splitPane{
		activeView: viewList,
		viewer:     newViewer(),
		histIdx:    -1,
	}
}

// openNote navigates to n, truncating any forward history.
func (sp *splitPane) openNote(n *vault.Note) {
	sp.searchReturn = nil
	if sp.histIdx >= 0 {
		sp.history = sp.history[:sp.histIdx+1]
	} else {
		sp.history = nil
	}
	sp.history = append(sp.history, n)
	sp.histIdx = len(sp.history) - 1
	sp.viewer = sp.viewer.withNote(n)
	sp.activeView = viewNote
}

// openNoteFromSearch opens n as the root of a fresh history, remembering
// results so a later "back" (once this note's own history is exhausted)
// restores the results list instead of whatever was open before the search.
func (sp *splitPane) openNoteFromSearch(n *vault.Note, results []*vault.Note) {
	sp.history = nil
	sp.histIdx = -1
	sp.openNote(n)
	sp.searchReturn = results
}

// back navigates to the previous note in history. Returns false if at start.
func (sp *splitPane) back() bool {
	if sp.histIdx <= 0 {
		return false
	}
	sp.histIdx--
	sp.viewer = sp.viewer.withNote(sp.history[sp.histIdx])
	sp.activeView = viewNote
	return true
}

// forward navigates to the next note in history. Returns false if at end.
func (sp *splitPane) forward() bool {
	if sp.histIdx >= len(sp.history)-1 {
		return false
	}
	sp.histIdx++
	sp.viewer = sp.viewer.withNote(sp.history[sp.histIdx])
	sp.activeView = viewNote
	return true
}

// currentNote returns the note currently shown (nil if on list view).
func (sp *splitPane) currentNote() *vault.Note {
	return sp.viewer.note
}
