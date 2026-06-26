package tui

import "github.com/charmbracelet/lipgloss"

type Theme struct {
	Name         string
	Accent       lipgloss.Color // bg of focused/active elements
	AccentFg     lipgloss.Color // foreground on Accent bg
	BorderFocus  lipgloss.Color // active/focused pane border
	BorderNormal lipgloss.Color // inactive pane border
	BorderPicker lipgloss.Color // pane picker highlight color
	StatusBg     lipgloss.Color // bottom status bar background
	DropdownBg   lipgloss.Color // command palette dropdown background
	TextPrimary  lipgloss.Color // main body text
	TextSecond   lipgloss.Color // secondary text
	TextMuted    lipgloss.Color // muted/dim text
	TextDim      lipgloss.Color // very dim text
	BlurredBg    lipgloss.Color // selection bg when pane is blurred
	Cursor       lipgloss.Color // cursor / highlight color
	Bg           lipgloss.Color // pane background ("" = terminal default)
}

var DarkTheme = Theme{
	Name:         "dark",
	Accent:       "62",
	AccentFg:     "230",
	BorderFocus:  "62",
	BorderNormal: "238",
	BorderPicker: "214",
	StatusBg:     "237",
	DropdownBg:   "235",
	TextPrimary:  "252",
	TextSecond:   "245",
	TextMuted:    "243",
	TextDim:      "240",
	BlurredBg:    "238",
	Cursor:       "214",
	Bg:           "",
}

var LightTheme = Theme{
	Name:         "light",
	Accent:       "25",
	AccentFg:     "255",
	BorderFocus:  "25",
	BorderNormal: "250",
	BorderPicker: "166",
	StatusBg:     "254",
	DropdownBg:   "255",
	TextPrimary:  "234",
	TextSecond:   "240",
	TextMuted:    "243",
	TextDim:      "246",
	BlurredBg:    "253",
	Cursor:       "166",
	Bg:           "255",
}

// activeTheme is the current theme used by all render functions.
var activeTheme = DarkTheme
