package tui

import (
	"fmt"
	"math"
	"strconv"
	"testing"
)

func TestThemesUseOneAccentAndReadableCorePairs(t *testing.T) {
	for _, theme := range ThemeChoices {
		t.Run(theme.Name, func(t *testing.T) {
			accent := string(theme.Accent)
			for role, got := range map[string]string{
				"BorderFocus":  string(theme.BorderFocus),
				"BorderPicker": string(theme.BorderPicker),
				"Cursor":       string(theme.Cursor),
			} {
				if got != accent {
					t.Errorf("%s = %s, want accent %s", role, got, accent)
				}
			}
			if ratio := contrastRatio(string(theme.TextPrimary), string(theme.Bg)); ratio < 4.5 {
				t.Errorf("primary/background contrast %.2f < 4.5", ratio)
			}
			if ratio := contrastRatio(accent, string(theme.Bg)); ratio < 3 {
				t.Errorf("accent/background contrast %.2f < 3", ratio)
			}
			if ratio := contrastRatio(string(theme.AccentFg), accent); ratio < 3 {
				t.Errorf("accent foreground contrast %.2f < 3", ratio)
			}
		})
	}
}

func TestThemeSuggestionsCoverEveryTheme(t *testing.T) {
	got := (paletteModel{}).themeSuggestions("")
	if len(got) != len(ThemeChoices) {
		t.Fatalf("theme suggestions = %d, want %d", len(got), len(ThemeChoices))
	}
}

func contrastRatio(a, b string) float64 {
	la, lb := relativeLuminance(a), relativeLuminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func relativeLuminance(hex string) float64 {
	if len(hex) != 7 || hex[0] != '#' {
		panic(fmt.Sprintf("unsupported color %q", hex))
	}
	channel := func(start int) float64 {
		value, err := strconv.ParseUint(hex[start:start+2], 16, 8)
		if err != nil {
			panic(err)
		}
		v := float64(value) / 255
		if v <= 0.04045 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	r, g, b := channel(1), channel(3), channel(5)
	return 0.2126*r + 0.7152*g + 0.0722*b
}
