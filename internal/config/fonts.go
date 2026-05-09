package config

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	figletFontsOnce sync.Once
	figletFonts     []string
	toiletFontsOnce sync.Once
	toiletFonts     []string
	fclkFontsOnce   sync.Once
	fclkFonts       []string
)

var fallbackFigletFonts = []string{"standard", "big", "block", "bubble", "digital", "mini", "script", "slant", "small"}

var fallbackToiletFonts = []string{"standard", "term", "future", "mono12", "smblock", "smmono12", "pagga", "emboss"}

func FigletFontChoices() []string {
	figletFontsOnce.Do(func() {
		figletFonts = discoverExternalFonts(fontDiscovery{
			Tool:       "figlet",
			Extensions: []string{".flf"},
			Fallbacks:  fallbackFigletFonts,
			Env:        []string{"FIGLET_FONTDIR", "FIGLETFONTDIR"},
			Names:      []string{"figlet"},
		})
	})
	return append([]string(nil), figletFonts...)
}

func ToiletFontChoices() []string {
	toiletFontsOnce.Do(func() {
		toiletFonts = discoverExternalFonts(fontDiscovery{
			Tool:       "toilet",
			Extensions: []string{".tlf", ".flf"},
			Fallbacks:  fallbackToiletFonts,
			Env:        []string{"TOILET_FONTDIR", "TOILETFONTDIR", "FIGLET_FONTDIR", "FIGLETFONTDIR"},
			Names:      []string{"toilet", "figlet"},
		})
	})
	return append([]string(nil), toiletFonts...)
}

func FclkFontChoices() []string {
	fclkFontsOnce.Do(func() {
		fclkFonts = discoverFclkFonts()
	})
	return append([]string(nil), fclkFonts...)
}

type fontDiscovery struct {
	Tool       string
	Extensions []string
	Fallbacks  []string
	Env        []string
	Names      []string
}

func discoverExternalFonts(spec fontDiscovery) []string {
	choices := make(map[string]bool)
	for _, fallback := range spec.Fallbacks {
		choices[fallback] = true
	}

	commandDirs := commandFontDirs(spec.Tool)
	for _, dir := range commandDirs {
		collectFonts(dir, spec.Extensions, false, choices)
	}
	for _, dir := range uniqueDirs(systemFontDirs(spec.Names), commandDirs) {
		collectFonts(dir, spec.Extensions, false, choices)
	}
	for _, dir := range uniqueDirs(envFontDirs(spec.Env), nil) {
		collectFonts(dir, spec.Extensions, true, choices)
	}
	for _, dir := range uniqueDirs(userFontDirs(spec.Names), nil) {
		collectFonts(dir, spec.Extensions, true, choices)
	}

	out := make([]string, 0, len(choices))
	for choice := range choices {
		out = append(out, choice)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i] == "standard" {
			return true
		}
		if out[j] == "standard" {
			return false
		}
		return strings.ToLower(out[i]) < strings.ToLower(out[j])
	})
	return out
}

func commandFontDirs(tool string) []string {
	if _, err := exec.LookPath(tool); err != nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	out, err := exec.CommandContext(ctx, tool, "-I2").Output()
	if err != nil {
		return nil
	}
	dir := strings.TrimSpace(string(out))
	if dir == "" {
		return nil
	}
	return []string{dir}
}

func uniqueDirs(candidates []string, seenDirs []string) []string {
	seen := make(map[string]bool)
	for _, dir := range seenDirs {
		if cleaned := cleanDir(dir); cleaned != "" {
			seen[cleaned] = true
		}
	}

	var dirs []string
	add := func(dir string) {
		cleaned := cleanDir(dir)
		if cleaned == "" || seen[cleaned] {
			return
		}
		seen[cleaned] = true
		dirs = append(dirs, cleaned)
	}

	for _, dir := range candidates {
		add(dir)
	}
	return dirs
}

func envFontDirs(names []string) []string {
	var dirs []string
	for _, name := range names {
		dirs = append(dirs, filepath.SplitList(os.Getenv(name))...)
	}
	return dirs
}

func systemFontDirs(names []string) []string {
	var dirs []string
	for _, base := range filepath.SplitList(os.Getenv("XDG_DATA_DIRS")) {
		for _, name := range names {
			dirs = append(dirs, filepath.Join(base, name))
		}
	}
	for _, prefix := range []string{"/usr/share", "/usr/local/share", "/opt/homebrew/share", "/opt/local/share"} {
		for _, name := range names {
			dirs = append(dirs, filepath.Join(prefix, name))
		}
	}
	if runtime.GOOS == "windows" {
		for _, base := range []string{os.Getenv("PROGRAMDATA")} {
			for _, name := range names {
				dirs = append(dirs, filepath.Join(base, name))
			}
		}
	}
	return dirs
}

func userFontDirs(names []string) []string {
	var dirs []string
	if base := os.Getenv("XDG_DATA_HOME"); base != "" {
		for _, name := range names {
			dirs = append(dirs, filepath.Join(base, name))
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		for _, name := range names {
			dirs = append(dirs,
				filepath.Join(home, "."+name),
				filepath.Join(home, ".local", "share", name),
				filepath.Join(home, "Library", "Application Support", name),
			)
		}
	}
	if runtime.GOOS == "windows" {
		for _, base := range []string{os.Getenv("APPDATA"), os.Getenv("LOCALAPPDATA")} {
			for _, name := range names {
				dirs = append(dirs, filepath.Join(base, name))
			}
		}
	}
	return dirs
}

func cleanDir(dir string) string {
	if dir == "" {
		return ""
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return filepath.Clean(dir)
	}
	return filepath.Clean(abs)
}

func collectFonts(root string, extensions []string, pathValues bool, choices map[string]bool) {
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return
	}

	exts := make(map[string]bool, len(extensions))
	for _, ext := range extensions {
		exts[strings.ToLower(ext)] = true
	}

	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if !exts[ext] {
			return nil
		}
		name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		if name == "" {
			return nil
		}
		if pathValues {
			choices[strings.TrimSuffix(path, filepath.Ext(path))] = true
		} else {
			choices[name] = true
		}
		return nil
	})
}

func discoverFclkFonts() []string {
	choices := make(map[string]bool)
	for _, dir := range uniqueDirs(fclkFontDirs(), nil) {
		collectFonts(dir, []string{".fclk"}, true, choices)
	}

	out := make([]string, 0, len(choices))
	for choice := range choices {
		out = append(out, choice)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(fontChoiceBase(out[i])) < strings.ToLower(fontChoiceBase(out[j]))
	})
	return out
}

func fclkFontDirs() []string {
	var dirs []string
	if base, err := os.UserConfigDir(); err == nil {
		dirs = append(dirs, filepath.Join(base, AppDir, "fonts"))
	}
	if cwd, err := os.Getwd(); err == nil {
		dirs = append(dirs, cwd)
	}
	return dirs
}

func fontChoiceBase(choice string) string {
	base := filepath.Base(choice)
	return strings.TrimSuffix(base, filepath.Ext(base))
}
