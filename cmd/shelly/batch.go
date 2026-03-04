package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/germanamz/shelly/pkg/engine"
)

func runBatch(args []string) error {
	fs := flag.NewFlagSet("batch", flag.ExitOnError)
	configPath := fs.String("config", "", "path to configuration file (default: .shelly/config.yaml or shelly.yaml)")
	shellyDir := fs.String("shelly-dir", ".shelly", "path to .shelly directory")
	tasksPath := fs.String("tasks", "", "path to input tasks JSONL file (required)")
	outputPath := fs.String("output", "", "path to output results JSONL file (required)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: shelly batch [flags]\n\nRun tasks in headless batch mode for cost-efficient processing.\n\nFlags:\n")
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nInput JSONL format (one task per line):\n")
		fmt.Fprintf(os.Stderr, "  {\"id\": \"task-1\", \"agent\": \"coder\", \"task\": \"Review this file\", \"context\": \"...\"}\n\n")
		fmt.Fprintf(os.Stderr, "Fields:\n")
		fmt.Fprintf(os.Stderr, "  id       Required. Unique task identifier.\n")
		fmt.Fprintf(os.Stderr, "  task     Required. The task prompt.\n")
		fmt.Fprintf(os.Stderr, "  agent    Optional. Agent name (default: entry_agent from config).\n")
		fmt.Fprintf(os.Stderr, "  context  Optional. Additional context prepended to the task.\n")
	}

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *tasksPath == "" {
		return fmt.Errorf("batch: --tasks flag is required")
	}
	if *outputPath == "" {
		return fmt.Errorf("batch: --output flag is required")
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer cancel()

	resolvedConfig := resolveConfigPath(*configPath, *shellyDir)

	cfg, err := engine.LoadConfig(resolvedConfig)
	if err != nil {
		return err
	}

	cfg.ShellyDir = *shellyDir
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

	fmt.Fprintf(os.Stderr, "Running batch: %s → %s\n", *tasksPath, *outputPath)

	if err := engine.RunBatch(ctx, eng, *tasksPath, *outputPath); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "Batch complete. Results written to %s\n", *outputPath)
	return nil
}
