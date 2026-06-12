# AGENTS.md

## Project Overview

`clk` is a Go Bubble Tea terminal clock app. The executable lives at `cmd/clk`, with implementation split across internal packages:

- `internal/app`: Bubble Tea model, key handling, Bubbles list settings overlays, layout, and clock composition.
- `internal/config`: YAML config schema, defaults, normalization, migration, load/save.
- `internal/render`: clock glyph renderers, seconds/progress renderers, figlet/toilet integration.
- `internal/theme`: built-in palettes and Lip Gloss styles.

The app uses Bubble Tea for the TUI runtime, Bubbles for help/progress UI, Lip Gloss for styling/layout, and `gopkg.in/yaml.v3` for config.

## User-Facing Behavior

- Default command: `go run ./cmd/clk`.
- Config path defaults to `os.UserConfigDir()/clk/config.yaml`, usually `~/.config/clk/config.yaml` on Linux.
- CLI flags:
  - `--config PATH`: override config path.
  - `--no-config`: run with defaults and skip config reads/writes.
  - `--version`: print the app version.
- Main keys:
  - `s`: settings overlay.
  - `?`: help overlay.
  - `q` / `ctrl+c`: quit.
  - Arrow keys or `h/j/k/l`: navigate/change settings.
  - `enter` / space: change selected setting.
  - `esc`: close overlay/back.

## Config And Settings

Current config fields include:

- `time.format`: `24h`, `12h`, or `utc`.
- `time.show_date`: show/hide date row.
- `time.timezone`: `Local` by default; named zones are accepted if Go can load them.
- `display.digit_style`: `block`, `braille`, `box`, `half_block`, `nerd_segment`, `figlet`, `toilet`.
- `display.figlet_font`: selected font for `figlet` style.
- `display.toilet_font`: selected font for `toilet` style.
- `display.seconds_style`: `hidden`, `numeric`, `progress_bar`, `bubble_progress`, `ascii_circle`, `braille_circle`, `nerd_pulse`, `pomodoro`, `workday`.
- `display.inline_seconds`: independent on/off toggle for rendering seconds in the main clock line.
- `display.size`: `normal` or `double`.
- `display.blink_separator`: hides separators on odd seconds using a width-stable hidden separator glyph.
- `display.alignment`: `left`, `center`, or `right`.
- `workday.start_time`: start of the configured workday, chosen from half-hour `HH:MM` values.
- `workday.end_time`: end of the configured workday, chosen from half-hour `HH:MM` values.
- `workday.days`: active workdays using `mon`, `tue`, `wed`, `thu`, `fri`, `sat`, `sun`; configured through the `Work days` checkbox submenu.
- `calendar.sources`: configured HTTP(S) `.ics` feeds. Legacy `calendar.url` values are migrated into this list.
- `calendar.mode`: `merged` combines all configured ICS sources into one progress row; `split` renders one progress row per source.
- `calendar.refresh_minutes`: fetch interval for all configured ICS sources.
- `calendar.sources[].last_event`: per-source remembered baseline event. Legacy top-level `calendar.last_event` is migrated to the matching source.
- `theme.name` and `theme.accent`: built-in theme palette and accent color.
- `ui.nerd_font`: enables Nerd Font-specific glyph choices where available.
- `ui.emoji`: switches compact progress labels from Unicode symbols to emoji symbols.

Normalization is intentionally conservative. Unknown enum values fall back to defaults. Old configs with `seconds_style: inline` are migrated to `inline_seconds: true` plus `seconds_style: hidden`.

## Rendering Notes

- Built-in clock glyphs are rendered per character so blinking separators can reserve exactly the same width as visible separators.
- Braille rendering is generated from pixel bitmaps into real Unicode braille cells. Do not replace it with `⣿`-style block approximations.
- Double-size braille scales the source bitmap before repacking into braille cells; naive rune duplication breaks the dot geometry.
- `figlet` and `toilet` styles are optional external renderers. If the selected command/font is unavailable, rendering falls back to the built-in clock. `figlet` style also tries `toilet -f <figlet_font>` as a compatible fallback when the `figlet` binary is absent.
- External glyphs are cached per command/font/rune to avoid spawning commands every frame for repeated characters.
- Layout uses `joinVerticalWithBackground` instead of raw centered `lipgloss.JoinVertical` where theme-background padding matters. This avoids transparent/default terminal whitespace around shorter rows.
- Third-party Bubbles output in overlays should be wrapped with `fillBackground` before and after panel rendering so internal padding and short lines inherit the active theme background. For Bubbles list menus, use the project-owned `settingsDelegate`; the default delegate emits internal ANSI resets that can leak terminal background.
- Bubble Progress seconds renderers should use the native Bubbles `progress.Model` flow with theme gradient colors; avoid hand-building the bar unless Bubbles cannot support a required behavior.
- Workday progress is 0% before the configured start time, fills to 100% between start and end, remains 100% after the end time, and resets to 0% on the next non-completed workday/off day.
- Workday, off-day, and ICS event progress rows use compact symbols instead of English status words. Emoji mode uses emoji symbols; non-emoji mode uses plain Unicode symbols. Off-work emoji rotate daily between beach, palm, and home symbols.
- ICS event titles should be truncated by terminal cell width and should use at most one third of the available progress-row width so the progress bar remains visible.

## Testing

Use Go's default build and module caches:

```sh
go test ./...
```

If the default Go caches are not writable in a sandboxed environment, use project-specific temp caches:

```sh
GOCACHE=/tmp/clk-go-build GOMODCACHE=/tmp/clk-go-mod go test ./...
```

Tests cover config normalization/migration, renderer output, width stability for blinking separators, settings/model behavior, theme fallbacks, and optional external font behavior. Tests that require `toilet` skip when it is not available.

## Development Guidance

- Prefer extending existing enums and settings items over adding one-off paths.
- Keep renderer behavior width-stable; avoid changes that make blinking separators or optional seconds display shift the main clock.
- Preserve theme backgrounds on all padding, borders, progress bars, and overlay joins.
- Keep settings submenus keyboard-driven and width-stable; main settings and workday checkboxes render through Bubbles list with model-owned selection state.
- Keep external command support optional and failure-tolerant.
- Run `gofmt -w internal cmd` and `go test ./...` before handing off changes.
