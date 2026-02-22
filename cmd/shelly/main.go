package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/germanamz/shelly/pkg/engine"
	"github.com/germanamz/shelly/pkg/shellydir"
)

func main() {
	// Handle subcommands before flag parsing.
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "init":
			initCmd := flag.NewFlagSet("init", flag.ExitOnError)
			initCmd.Usage = func() {
				fmt.Fprintf(os.Stderr, "Usage: shelly init [flags]\n\nInitialize a .shelly directory with default structure and config.\n\nFlags:\n")
				initCmd.PrintDefaults()
			}
			dir := initCmd.String("shelly-dir", ".shelly", "path to .shelly directory")
			_ = initCmd.Parse(os.Args[2:])

			if err := runInit(*dir); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}

			return
		}
	}

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: shelly [flags]\n       shelly <command> [flags]\n\nFlags:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nCommands:\n  init    Initialize a .shelly directory with default structure and config\n")
	}

	configPath := flag.String("config", "", "path to configuration file (default: .shelly/config.yaml or shelly.yaml)")
	shellyDir := flag.String("shelly-dir", ".shelly", "path to .shelly directory")
	envFile := flag.String("env", ".env", "path to .env file (ignored if missing)")
	agentName := flag.String("agent", "", "agent to start with (overrides entry_agent in config)")
	verbose := flag.Bool("verbose", false, "show tool results and thinking text")
	flag.Parse()

	if err := loadDotEnv(*envFile); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if err := run(*configPath, *shellyDir, *agentName, *verbose); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func runInit(dirPath string) error {
	d := shellydir.New(dirPath)

	configYAML, err := runWizard()
	if err != nil {
		return err
	}

	if err := shellydir.BootstrapWithConfig(d, configYAML); err != nil {
		return err
	}

	fmt.Printf("Initialized %s\n", d.Root())

	return nil
}

func run(configPath, shellyDirPath, agentName string, verbose bool) error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Config resolution: explicit flag → .shelly/config.yaml → shelly.yaml.
	resolvedConfig := resolveConfigPath(configPath, shellyDirPath)

	cfg, err := engine.LoadConfig(resolvedConfig)
	if err != nil {
		return err
	}

	cfg.ShellyDir = shellyDirPath

	eng, err := engine.New(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() { _ = eng.Close() }()

	sess, err := eng.NewSession(agentName)
	if err != nil {
		return err
	}

	model := newAppModel(ctx, sess, eng.Events(), verbose)

	p := tea.NewProgram(
		model,
		tea.WithAltScreen(),
	)

	// Send the program reference so the model can start bridge goroutines.
	go func() {
		p.Send(programReadyMsg{program: p})
	}()

	_, err = p.Run()
	return err
}
