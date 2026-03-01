package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/germanamz/shelly/cmd/shelly/internal/app"
	"github.com/germanamz/shelly/cmd/shelly/internal/format"
	"github.com/germanamz/shelly/cmd/shelly/internal/msgs"
	"github.com/germanamz/shelly/cmd/shelly/internal/templates"
	"github.com/germanamz/shelly/cmd/shelly/internal/tty"
	"github.com/germanamz/shelly/pkg/engine"
	"github.com/germanamz/shelly/pkg/projectctx"
	"github.com/germanamz/shelly/pkg/shellydir"
)

func runIndex(args []string) error {
	fs := flag.NewFlagSet("index", flag.ExitOnError)
	shellyDirPath := fs.String("shelly-dir", ".shelly", "path to .shelly directory")
	check := fs.Bool("check", false, "only check if the knowledge graph is stale (no indexing)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	dir := shellydir.New(*shellyDirPath)
	projectRoot := filepath.Dir(dir.Root())

	if *check {
		if projectctx.IsKnowledgeStale(projectRoot, dir) {
			fmt.Println("Knowledge graph is stale. Run 'shelly index' to refresh.")
			os.Exit(1)
		}
		fmt.Println("Knowledge graph is up to date.")
		return nil
	}

	// Load the project-indexer template.
	tmpl, err := templates.Get("project-indexer")
	if err != nil {
		return fmt.Errorf("index: %w", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Ensure .shelly/ structure exists.
	if !dir.Exists() {
		return fmt.Errorf("index: .shelly/ directory not found at %s (run 'shelly init' first)", *shellyDirPath)
	}

	// Build engine config from the indexer template.
	cfg := tmpl.Config
	engine.ExpandConfigStrings(&cfg)
	cfg.ShellyDir = *shellyDirPath
	cfg.StatusFunc = func(msg string) {
		fmt.Fprintf(os.Stderr, "\r\033[K  %s", msg)
	}

	// Write embedded skills to disk so they are available to agents.
	for _, sk := range tmpl.EmbeddedSkills {
		skillDir := filepath.Join(dir.SkillsDir(), sk.Name)
		if err := os.MkdirAll(skillDir, 0o750); err != nil {
			return fmt.Errorf("index: create skill dir %q: %w", sk.Name, err)
		}
		skillPath := filepath.Join(skillDir, "SKILL.md")
		// Only write if missing.
		if _, err := os.Stat(skillPath); os.IsNotExist(err) {
			if err := os.WriteFile(skillPath, []byte(sk.Content), 0o644); err != nil { //nolint:gosec // skill content, not secret
				return fmt.Errorf("index: write skill %q: %w", sk.Name, err)
			}
		}
	}

	eng, err := engine.New(ctx, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr)
		return err
	}
	fmt.Fprintln(os.Stderr)
	defer func() { _ = eng.Close() }()

	sess, err := eng.NewSession("")
	if err != nil {
		return err
	}

	model := app.NewAppModel(ctx, sess, eng)
	model.InitialMessage = "Index this project. Build or update the knowledge graph in .shelly/."

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

	go func() {
		p.Send(msgs.ProgramReadyMsg{Program: p})
	}()

	_, err = p.Run()
	return err
}
