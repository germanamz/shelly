package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/germanamz/shelly/cmd/shelly/internal/configwizard"
	"github.com/germanamz/shelly/pkg/engine"
)

func runConfig(args []string) error {
	fs := flag.NewFlagSet("config", flag.ExitOnError)
	shellyDir := fs.String("shelly-dir", ".shelly", "path to .shelly directory")
	configPath := fs.String("config", "", "path to configuration file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Resolve config path.
	resolved := resolveConfigPath(*configPath, *shellyDir)

	// Try to load existing config (raw, without env expansion).
	var cfg engine.Config
	if _, err := os.Stat(resolved); err == nil {
		loaded, loadErr := engine.LoadConfigRaw(resolved)
		if loadErr != nil {
			return fmt.Errorf("loading existing config: %w", loadErr)
		}
		cfg = loaded
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("checking config file: %w", err)
	}

	model := configwizard.NewWizardModel(cfg, *configPath, *shellyDir)
	p := tea.NewProgram(model)

	_, err := p.Run()
	return err
}
