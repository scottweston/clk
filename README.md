# clk

`clk` is a Bubble Tea terminal clock with large digital renderers, themeable colors,
animated seconds, optional inline seconds, Bubble Tea progress bars,
Pomodoro, workday, and ICS calendar progress, blinking separators, double-size rendering,
optional figlet/toilet and `.fclk` font rendering, and YAML configuration saved
under the user config directory.

## Usage

```sh
go run ./cmd/clk
```

Keybindings:

- `s`: open settings
- `?`: help
- `q` / `ctrl+c`: quit

Config is stored at `~/.config/clk/config.yaml` on Linux unless `--config` or
`--no-config` is provided.

User-defined `.fclk` clock fonts are discovered from the top level of
`~/.config/clk` and the current working directory. Subdirectories are not
searched. Select the `fclk` digit style in settings, then use the `FCLK font`
selector to choose a discovered font.

Workday progress uses configurable start/end times and a `Work days` checkbox
submenu in settings. Workday and ICS calendar progress bars can be enabled
independently in settings; set the `ICS URLs` value to one HTTP(S) `.ics` feed
per line for calendar countdown/countup progress. `ICS mode` controls whether
feeds are `merged` into one combined progress bar or `split` into one progress
bar per feed. Progress labels use compact Unicode symbols by default, with an
`Emoji` setting and `ui.emoji` config flag for emoji symbols. Off-work emoji
rotate daily between beach, palm, and home symbols.

The calendar countdown normally uses the most recent completed event from each
feed as its baseline, and remembers that event in config in case a feed later
purges old entries. To manually seed that baseline, add a `last_event` block to
the matching source:

```yaml
calendar:
  show_progress: true
  mode: merged
  refresh_minutes: 15
  sources:
    - url: https://example.com/work.ics
      last_event:
        source_url: https://example.com/work.ics
        summary: Standup
        start: 2026-05-01T09:00:00-04:00
        end: 2026-05-01T09:30:00-04:00
    - url: https://example.com/personal.ics
```

`source_url` should match the source `url` and may be omitted inside source-level
`last_event` blocks. `start`/`end` should be RFC3339 timestamps with an offset
or `Z`. Invalid or mismatched `last_event` values are ignored, and the app falls
back to launch time until it sees a completed event. Older configs using
`calendar.url` and a top-level `calendar.last_event` are migrated into
`calendar.sources` when loaded.
