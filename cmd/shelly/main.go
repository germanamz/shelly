package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
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
			tmpl := initCmd.String("template", "", "use a config template (use \"list\" to see available templates)")
			_ = initCmd.Parse(os.Args[2:])

			if err := runInit(*dir, *tmpl); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}

			return
		case "config":
			configCmd := flag.NewFlagSet("config", flag.ExitOnError)
			configCmd.Usage = func() {
				fmt.Fprintf(os.Stderr, "Usage: shelly config [flags]\n\nEdit an existing config file interactively.\n\nFlags:\n")
				configCmd.PrintDefaults()
			}
			cfgPath := configCmd.String("config", "", "path to configuration file")
			dir := configCmd.String("shelly-dir", ".shelly", "path to .shelly directory")
			_ = configCmd.Parse(os.Args[2:])

			if err := runConfigEditor(*cfgPath, *dir); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}

			return
		}
	}

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: shelly [flags]\n       shelly <command> [flags]\n\nFlags:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nCommands:\n  init    Initialize a .shelly directory with default structure and config\n  config  Edit an existing config file interactively\n")
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

func runInit(dirPath, templateName string) error {
	if templateName == "list" {
		printTemplateList()
		return nil
	}

	var (
		result wizardResult
		err    error
	)

	if templateName != "" {
		tmpl := findTemplate(templateName)
		if tmpl == nil {
			return fmt.Errorf("unknown template %q (use --template list to see available templates)", templateName)
		}

		result, err = runTemplateWizard(tmpl)
	} else {
		result, err = runWizard()
	}

	if err != nil {
		return err
	}

	d := shellydir.New(dirPath)

	if err := shellydir.BootstrapWithConfig(d, result.ConfigYAML); err != nil {
		return err
	}

	if err := writeSkillFiles(d, result.SkillFiles); err != nil {
		return err
	}

	fmt.Printf("Initialized %s\n", d.Root())

	return nil
}

// writeSkillFiles creates skill folders and SKILL.md files in the skills directory.
// Existing files are not overwritten.
func writeSkillFiles(d shellydir.Dir, files []skillFile) error {
	for _, sf := range files {
		skillDir := filepath.Join(d.SkillsDir(), sf.Name)

		if err := os.MkdirAll(skillDir, 0o750); err != nil {
			return fmt.Errorf("create skill dir %q: %w", sf.Name, err)
		}

		skillPath := filepath.Join(skillDir, "SKILL.md")

		// Do not overwrite existing skill files.
		if _, err := os.Stat(skillPath); err == nil {
			continue
		}

		if err := os.WriteFile(skillPath, []byte(sf.Content), 0o600); err != nil {
			return fmt.Errorf("write skill %q: %w", sf.Name, err)
		}
	}

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

	model := newAppModel(ctx, sess, eng, verbose)

	p := tea.NewProgram(model)

	// Send the program reference so the model can start bridge goroutines.
	go func() {
		p.Send(programReadyMsg{program: p})
	}()

	_, err = p.Run()
	return err
}
