# clk

`clk` is a Bubble Tea terminal clock with large digital renderers, themeable colors,
animated seconds, optional inline seconds, Bubble Tea progress bars,
Pomodoro progress, blinking separators, double-size rendering, optional
figlet/toilet font rendering, and YAML configuration saved under the user config
directory.

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
