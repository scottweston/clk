package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"clk/internal/app"
	"clk/internal/config"
)

const version = "0.1.0"

func main() {
	configPath := flag.String("config", "", "path to config file")
	noConfig := flag.Bool("no-config", false, "run without reading or writing config")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	manager := config.NewManager(*configPath, *noConfig)
	cfg, err := manager.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "clk: %v\n", err)
		os.Exit(1)
	}

	program := tea.NewProgram(app.New(cfg, manager), tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		if !errors.Is(err, tea.ErrProgramKilled) {
			fmt.Fprintf(os.Stderr, "clk: %v\n", err)
			os.Exit(1)
		}
	}
}
