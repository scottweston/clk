# TODO

## Product

- Add i18n support for dates, settings labels, help text, and status messages.
- Add locale-aware time/date formatting while keeping explicit `24h`, `12h`, and `utc` modes predictable.
- Add optional presets for common clock layouts, such as minimal, workday, pomodoro, and large display.
- Add import/export or reset flows for configuration from the settings overlay.

## Rendering

- Make progress bar empty-fill foreground/background choices more explicit in config and settings.
- Add previews for digit styles, seconds styles, and external fonts inside settings.
- Improve fallback messaging when `figlet` or `toilet` is unavailable without disrupting the clock view.
- Audit renderer width stability whenever new glyph styles or seconds displays are added.

## Configuration

- Consider config version migration helpers before adding fields that need non-trivial migrations.
- Add validation coverage for user-facing config values that depend on the active theme or environment.
- Consider a documented config schema example for users who prefer editing YAML directly.

## Testing

- Add golden tests for representative themed views, including settings and help overlays.
- Add tests for progress bar ANSI styling so foreground/background regressions are easier to catch.
- Add integration coverage for save/load behavior using real temporary config files.

## Distribution

- Add release automation for tagged builds.
- Add installation notes for optional `figlet`, `toilet`, and Nerd Font support.
- Consider packaging metadata for common package managers after the CLI surface stabilizes.
