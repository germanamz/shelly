package main

import (
	"flag"
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/germanamz/shelly/cmd/shelly/internal/configwizard"
	"github.com/germanamz/shelly/cmd/shelly/internal/templates"
)

func runInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	shellyDir := fs.String("shelly-dir", ".shelly", "path to .shelly directory")
	templateName := fs.String("template", "", "template name (non-interactive)")
	force := fs.Bool("force", false, "overwrite existing config and skills")
	list := fs.Bool("list", false, "list available templates")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *list {
		metas := templates.List()
		if len(metas) == 0 {
			fmt.Println("No templates available")
			return nil
		}
		for _, m := range metas {
			fmt.Printf("  %-20s %s\n", m.Name, m.Description)
		}
		return nil
	}

	if *templateName != "" {
		t, err := templates.Get(*templateName)
		if err != nil {
			return err
		}
		if err := templates.Apply(t, *shellyDir, *force); err != nil {
			return err
		}
		fmt.Printf("Initialized %q template in %s\n", *templateName, *shellyDir)
		fmt.Println("Run 'shelly index' to build the project knowledge graph.")
		return nil
	}

	// Interactive mode.
	model := configwizard.NewTemplatePickerModel(*shellyDir, *force)
	p := tea.NewProgram(model)
	_, err := p.Run()
	return err
}
