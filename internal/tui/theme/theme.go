package theme

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// Catppuccin Mocha palette — same family as memoria's theme.
var (
	ColorRosewater = lipgloss.Color("#f5e0dc")
	ColorFlamingo  = lipgloss.Color("#f2cdcd")
	ColorPink      = lipgloss.Color("#f5c2e7")
	ColorMauve     = lipgloss.Color("#cba6f7")
	ColorRed       = lipgloss.Color("#f38ba8")
	ColorMaroon    = lipgloss.Color("#eba0ac")
	ColorPeach     = lipgloss.Color("#fab387")
	ColorYellow    = lipgloss.Color("#f9e2af")
	ColorGreen     = lipgloss.Color("#a6e3a1")
	ColorTeal      = lipgloss.Color("#94e2d5")
	ColorSky       = lipgloss.Color("#89dceb")
	ColorSapphire  = lipgloss.Color("#74c7ec")
	ColorBlue      = lipgloss.Color("#89b4fa")
	ColorLavender  = lipgloss.Color("#b4befe")

	ColorText     = lipgloss.Color("#cdd6f4")
	ColorSubtext1 = lipgloss.Color("#bac2de")
	ColorSubtext0 = lipgloss.Color("#a6adc8")
	ColorOverlay2 = lipgloss.Color("#9399b2")
	ColorOverlay1 = lipgloss.Color("#7f849c")
	ColorOverlay0 = lipgloss.Color("#6c7086")
	ColorSurface2 = lipgloss.Color("#585b70")
	ColorSurface1 = lipgloss.Color("#45475a")
	ColorSurface0 = lipgloss.Color("#313244")
	ColorBase     = lipgloss.Color("#1e1e2e")
	ColorMantle   = lipgloss.Color("#181825")
	ColorCrust    = lipgloss.Color("#11111b")
)

// Semantic aliases used throughout the TUI.
var (
	ActiveTab   = ColorMauve
	InactiveTab = ColorOverlay1
	Selected    = ColorMauve
	Dimmed      = ColorOverlay0
	Border      = ColorSurface1
	StatusOK    = ColorGreen
	StatusError = ColorRed
	ReasonColor = ColorPeach
	RepoColor   = ColorBlue
	TimeColor   = ColorOverlay1
)

// ReasonColorFor returns a distinct color for each notification reason,
// making it easy to visually distinguish why you received a notification.
func ReasonColorFor(reason string) color.Color {
	switch reason {
	case "review_requested":
		return ColorMauve
	case "mention", "team_mention":
		return ColorYellow
	case "assign":
		return ColorGreen
	case "author":
		return ColorBlue
	case "comment":
		return ColorTeal
	case "ci_activity":
		return ColorPeach
	case "security_alert":
		return ColorRed
	case "state_change":
		return ColorSky
	case "approval_requested":
		return ColorPink
	default:
		return ColorOverlay1 // subscribed, manual, etc.
	}
}
