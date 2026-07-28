package tui

import "github.com/charmbracelet/lipgloss"

// Theme defines one accent for location/focus plus a neutral hierarchy.
// Additional hues must never be assigned to interaction-highlight roles.
type Theme struct {
	// Name is the stable persisted theme identifier.
	Name string
	// Accent marks the current row, section, pane, picker, and cursor.
	Accent lipgloss.Color
	// AccentFg is readable text placed on an Accent background.
	AccentFg lipgloss.Color
	// BorderFocus marks the focused pane and always equals Accent.
	BorderFocus lipgloss.Color
	// BorderNormal separates inactive panes using a neutral tone.
	BorderNormal lipgloss.Color
	// BorderPicker marks the pane-picker target and always equals Accent.
	BorderPicker lipgloss.Color
	// StatusBg is the neutral background of the bottom status bar.
	StatusBg lipgloss.Color
	// DropdownBg is the neutral background behind command suggestions.
	DropdownBg lipgloss.Color
	// TextPrimary is the highest-contrast neutral for body content.
	TextPrimary lipgloss.Color
	// TextSecond is the second neutral step for supporting content.
	TextSecond lipgloss.Color
	// TextMuted is the third neutral step for de-emphasized content.
	TextMuted lipgloss.Color
	// TextDim is the fourth neutral step for the least prominent content.
	TextDim lipgloss.Color
	// BlurredBg keeps selection visible in an unfocused pane without using Accent.
	BlurredBg lipgloss.Color
	// Cursor marks the character cursor and always equals Accent.
	Cursor lipgloss.Color
	// Bg is the base pane background.
	Bg lipgloss.Color
}

var NordTheme = Theme{
	Name: "nord", Accent: "#81A1C1", AccentFg: "#2E3440",
	BorderFocus: "#81A1C1", BorderNormal: "#4C566A", BorderPicker: "#81A1C1",
	StatusBg: "#3B4252", DropdownBg: "#2E3440",
	TextPrimary: "#ECEFF4", TextSecond: "#D8DEE9", TextMuted: "#A8B2C1", TextDim: "#69788F",
	BlurredBg: "#3B4252", Cursor: "#81A1C1", Bg: "#2E3440",
}

var SolarizedDarkTheme = Theme{
	Name: "solarized", Accent: "#2AA198", AccentFg: "#002B36",
	BorderFocus: "#2AA198", BorderNormal: "#586E75", BorderPicker: "#2AA198",
	StatusBg: "#073642", DropdownBg: "#002B36",
	TextPrimary: "#FDF6E3", TextSecond: "#EEE8D5", TextMuted: "#93A1A1", TextDim: "#657B83",
	BlurredBg: "#073642", Cursor: "#2AA198", Bg: "#002B36",
}

var DraculaTheme = Theme{
	Name: "dracula", Accent: "#BD93F9", AccentFg: "#282A36",
	BorderFocus: "#BD93F9", BorderNormal: "#6272A4", BorderPicker: "#BD93F9",
	StatusBg: "#44475A", DropdownBg: "#282A36",
	TextPrimary: "#F8F8F2", TextSecond: "#D7D7D2", TextMuted: "#A6A6A1", TextDim: "#6F7182",
	BlurredBg: "#44475A", Cursor: "#BD93F9", Bg: "#282A36",
}

var GruvboxTheme = Theme{
	Name: "gruvbox", Accent: "#83A598", AccentFg: "#282828",
	BorderFocus: "#83A598", BorderNormal: "#665C54", BorderPicker: "#83A598",
	StatusBg: "#3C3836", DropdownBg: "#282828",
	TextPrimary: "#FBF1C7", TextSecond: "#EBDBB2", TextMuted: "#BDAE93", TextDim: "#7C6F64",
	BlurredBg: "#3C3836", Cursor: "#83A598", Bg: "#282828",
}

var TokyoNightTheme = Theme{
	Name: "tokyonight", Accent: "#7AA2F7", AccentFg: "#1A1B26",
	BorderFocus: "#7AA2F7", BorderNormal: "#565F89", BorderPicker: "#7AA2F7",
	StatusBg: "#24283B", DropdownBg: "#1A1B26",
	TextPrimary: "#C0CAF5", TextSecond: "#A9B1D6", TextMuted: "#787C99", TextDim: "#565F89",
	BlurredBg: "#24283B", Cursor: "#7AA2F7", Bg: "#1A1B26",
}

var SolarizedLightTheme = Theme{
	Name: "solarized-light", Accent: "#268BD2", AccentFg: "#FDF6E3",
	BorderFocus: "#268BD2", BorderNormal: "#93A1A1", BorderPicker: "#268BD2",
	StatusBg: "#EEE8D5", DropdownBg: "#FDF6E3",
	TextPrimary: "#586E75", TextSecond: "#657B83", TextMuted: "#839496", TextDim: "#93A1A1",
	BlurredBg: "#EEE8D5", Cursor: "#268BD2", Bg: "#FDF6E3",
}

var CatppuccinMochaTheme = Theme{
	Name: "catppuccin-mocha", Accent: "#89B4FA", AccentFg: "#1E1E2E",
	BorderFocus: "#89B4FA", BorderNormal: "#585B70", BorderPicker: "#89B4FA",
	StatusBg: "#313244", DropdownBg: "#1E1E2E",
	TextPrimary: "#CDD6F4", TextSecond: "#BAC2DE", TextMuted: "#9399B2", TextDim: "#6C7086",
	BlurredBg: "#313244", Cursor: "#89B4FA", Bg: "#1E1E2E",
}

var EverforestTheme = Theme{
	Name: "everforest", Accent: "#A7C080", AccentFg: "#2D353B",
	BorderFocus: "#A7C080", BorderNormal: "#7A8478", BorderPicker: "#A7C080",
	StatusBg: "#343F44", DropdownBg: "#2D353B",
	TextPrimary: "#D3C6AA", TextSecond: "#B7B89A", TextMuted: "#9DA9A0", TextDim: "#7A8478",
	BlurredBg: "#343F44", Cursor: "#A7C080", Bg: "#2D353B",
}

// ThemeChoices is the stable persisted ordering shown in Configuration.
var ThemeChoices = []Theme{
	NordTheme,
	SolarizedDarkTheme,
	DraculaTheme,
	GruvboxTheme,
	TokyoNightTheme,
	SolarizedLightTheme,
	CatppuccinMochaTheme,
	EverforestTheme,
}

var activeTheme = NordTheme
