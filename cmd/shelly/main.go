package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/germanamz/shelly/cmd/shelly/internal/app"
	"github.com/germanamz/shelly/cmd/shelly/internal/format"
	"github.com/germanamz/shelly/cmd/shelly/internal/msgs"
	"github.com/germanamz/shelly/cmd/shelly/internal/tty"
	"github.com/germanamz/shelly/pkg/engine"
)

func main() {
	// Subcommand dispatch: check os.Args before flag.Parse().
	if len(os.Args) > 1 && os.Args[1] == "config" {
		if err := runConfig(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: shelly [command] [flags]\n\nCommands:\n  config    Interactive configuration wizard\n\nFlags:\n")
		flag.PrintDefaults()
	}

	configPath := flag.String("config", "", "path to configuration file (default: .shelly/config.yaml or shelly.yaml)")
	shellyDir := flag.String("shelly-dir", ".shelly", "path to .shelly directory")
	envFile := flag.String("env", ".env", "path to .env file (ignored if missing)")
	agentName := flag.String("agent", "", "agent to start with (overrides entry_agent in config)")
	flag.Parse()

	if err := loadDotEnv(*envFile); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if err := run(*configPath, *shellyDir, *agentName); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(configPath, shellyDirPath, agentName string) error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Config resolution: explicit flag → .shelly/config.yaml → shelly.yaml.
	resolvedConfig := resolveConfigPath(configPath, shellyDirPath)

	cfg, err := engine.LoadConfig(resolvedConfig)
	if err != nil {
		return err
	}

	cfg.ShellyDir = shellyDirPath
	cfg.StatusFunc = func(msg string) {
		fmt.Fprintf(os.Stderr, "\r\033[K  %s", msg)
	}

	eng, err := engine.New(ctx, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr)
		return err
	}
	fmt.Fprintln(os.Stderr)
	defer func() { _ = eng.Close() }()

	sess, err := eng.NewSession(agentName)
	if err != nil {
		return err
	}

	model := app.NewAppModel(ctx, sess, eng)

	// Force the OSC 11 background-color query and consume its response
	// synchronously before bubbletea starts. This prevents the response from
	// leaking into the textarea as garbage text when the virtual cursor
	// triggers the same query later. The result is stored so glamour can use
	// a fixed style without issuing its own query.
	format.IsDarkBG = lipgloss.HasDarkBackground(os.Stdin, os.Stdout)
	tty.FlushStdinBuffer()

	staleFilter := tty.NewStaleEscapeFilter(func(m tea.Model) bool {
		switch v := m.(type) {
		case app.AppModel:
			return v.InputEnabled()
		case *app.AppModel:
			return v.InputEnabled()
		default:
			return true
		}
	})
	p := tea.NewProgram(model, tea.WithFilter(staleFilter))

	// Send the program reference so the model can start bridge goroutines.
	go func() {
		p.Send(msgs.ProgramReadyMsg{Program: p})
	}()

	_, err = p.Run()
	return err
}
