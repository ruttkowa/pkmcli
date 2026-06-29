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

var NordTheme = Theme{
	Name:         "nord",
	Accent:       "#5E81AC",
	AccentFg:     "#ECEFF4",
	BorderFocus:  "#5E81AC",
	BorderNormal: "#3B4252",
	BorderPicker: "#EBCB8B",
	StatusBg:     "#3B4252",
	DropdownBg:   "#2E3440",
	TextPrimary:  "#D8DEE9",
	TextSecond:   "#B0C8D8",
	TextMuted:    "#7B90A0",
	TextDim:      "#4C566A",
	BlurredBg:    "#3B4252",
	Cursor:       "#EBCB8B",
	Bg:           "#2E3440",
}

var SolarizedDarkTheme = Theme{
	Name:         "solarized",
	Accent:       "#268BD2",
	AccentFg:     "#FDF6E3",
	BorderFocus:  "#268BD2",
	BorderNormal: "#0A3848",
	BorderPicker: "#CB4B16",
	StatusBg:     "#073642",
	DropdownBg:   "#002B36",
	TextPrimary:  "#839496",
	TextSecond:   "#718090",
	TextMuted:    "#586E75",
	TextDim:      "#405060",
	BlurredBg:    "#073642",
	Cursor:       "#CB4B16",
	Bg:           "#002B36",
}

var DraculaTheme = Theme{
	Name:         "dracula",
	Accent:       "#BD93F9",
	AccentFg:     "#F8F8F2",
	BorderFocus:  "#BD93F9",
	BorderNormal: "#44475A",
	BorderPicker: "#FFB86C",
	StatusBg:     "#44475A",
	DropdownBg:   "#282A36",
	TextPrimary:  "#F8F8F2",
	TextSecond:   "#BFBFBF",
	TextMuted:    "#6272A4",
	TextDim:      "#44475A",
	BlurredBg:    "#3C3F4F",
	Cursor:       "#FFB86C",
	Bg:           "#282A36",
}

var GruvboxTheme = Theme{
	Name:         "gruvbox",
	Accent:       "#458588",
	AccentFg:     "#FBF1C7",
	BorderFocus:  "#458588",
	BorderNormal: "#504945",
	BorderPicker: "#D65D0E",
	StatusBg:     "#3C3836",
	DropdownBg:   "#282828",
	TextPrimary:  "#EBDBB2",
	TextSecond:   "#BDAE93",
	TextMuted:    "#928374",
	TextDim:      "#504945",
	BlurredBg:    "#3C3836",
	Cursor:       "#D65D0E",
	Bg:           "#282828",
}

var TokyoNightTheme = Theme{
	Name:         "tokyonight",
	Accent:       "#7AA2F7",
	AccentFg:     "#1A1B26",
	BorderFocus:  "#7AA2F7",
	BorderNormal: "#24283B",
	BorderPicker: "#F7768E",
	StatusBg:     "#16161E",
	DropdownBg:   "#1A1B26",
	TextPrimary:  "#A9B1D6",
	TextSecond:   "#787C99",
	TextMuted:    "#565F89",
	TextDim:      "#3A3F60",
	BlurredBg:    "#24283B",
	Cursor:       "#F7768E",
	Bg:           "#1A1B26",
}

// ThemeChoices is the ordered list of available themes.
var ThemeChoices = []Theme{
	NordTheme,
	SolarizedDarkTheme,
	DraculaTheme,
	GruvboxTheme,
	TokyoNightTheme,
}

// activeTheme is the current theme used by all render functions.
var activeTheme = NordTheme
