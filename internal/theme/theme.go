package theme

import "github.com/charmbracelet/lipgloss"

type Palette struct {
	Name       string
	Foreground string
	Muted      string
	Background string
	Accent     map[string]string
	Warning    string
}

func All() []Palette {
	return []Palette{
		palette("tokyo-night", "#c0caf5", "#565f89", "#1a1b26", "#f7768e"),
		palette("catppuccin", "#cdd6f4", "#6c7086", "#1e1e2e", "#f38ba8"),
		palette("nord", "#eceff4", "#81a1c1", "#2e3440", "#bf616a"),
		palette("dracula", "#f8f8f2", "#6272a4", "#282a36", "#ff5555"),
		palette("gruvbox", "#ebdbb2", "#928374", "#282828", "#fb4934"),
		palette("solarized", "#eee8d5", "#839496", "#002b36", "#dc322f"),
		palette("monokai", "#f8f8f2", "#75715e", "#272822", "#f92672"),
		palette("rose-pine", "#e0def4", "#908caa", "#191724", "#eb6f92"),
		palette("everforest", "#d3c6aa", "#859289", "#2b3339", "#e67e80"),
		palette("paper", "#202124", "#6f7378", "#fafafa", "#b3261e"),
	}
}

func Names() []string {
	values := All()
	names := make([]string, 0, len(values))
	for _, p := range values {
		names = append(names, p.Name)
	}
	return names
}

func Lookup(name string) Palette {
	for _, p := range All() {
		if p.Name == name {
			return p
		}
	}
	return All()[0]
}

func Accent(p Palette, name string) string {
	if color, ok := p.Accent[name]; ok {
		return color
	}
	return p.Accent["cyan"]
}

func Styles(p Palette, accentName string) Stylesheet {
	accent := Accent(p, accentName)
	return Stylesheet{
		Clock: lipgloss.NewStyle().
			Foreground(lipgloss.Color(accent)).
			Background(lipgloss.Color(p.Background)).
			Bold(true),
		Date: lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.Muted)).
			Background(lipgloss.Color(p.Background)),
		Seconds: lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.Foreground)).
			Background(lipgloss.Color(p.Background)),
		Panel: lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.Foreground)).
			Background(lipgloss.Color(p.Background)).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(accent)).
			BorderBackground(lipgloss.Color(p.Background)).
			Padding(1, 2),
		Selected: lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.Background)).
			Background(lipgloss.Color(accent)).
			Bold(true),
		Muted: lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.Muted)).
			Background(lipgloss.Color(p.Background)),
		Error: lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.Warning)).
			Background(lipgloss.Color(p.Background)),
	}
}

type Stylesheet struct {
	Clock    lipgloss.Style
	Date     lipgloss.Style
	Seconds  lipgloss.Style
	Panel    lipgloss.Style
	Selected lipgloss.Style
	Muted    lipgloss.Style
	Error    lipgloss.Style
}

func palette(name, fg, muted, bg, warning string) Palette {
	return Palette{
		Name:       name,
		Foreground: fg,
		Muted:      muted,
		Background: bg,
		Warning:    warning,
		Accent: map[string]string{
			"cyan":   "#7dcfff",
			"green":  "#9ece6a",
			"pink":   "#ff79c6",
			"purple": "#bb9af7",
			"yellow": "#e0af68",
			"orange": "#ff9e64",
			"red":    warning,
			"blue":   "#7aa2f7",
		},
	}
}
