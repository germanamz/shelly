package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/germanamz/shelly/pkg/projectctx"
	"github.com/germanamz/shelly/pkg/shellydir"
	"github.com/germanamz/shelly/pkg/skill"
)

// parallelInit runs skills loading, project context loading, and MCP server
// connections concurrently to reduce startup latency.
func (e *Engine) parallelInit(ctx context.Context, cfg Config, dir shellydir.Dir, status func(string)) error {
	var (
		skillsErr      error
		mcpErr         error
		loadedSkills   []skill.Skill
		loadedCtx      projectctx.Context
		knowledgeStale bool
		wg             sync.WaitGroup
	)

	skillsDir := dir.SkillsDir()
	if _, err := os.Stat(skillsDir); err == nil {
		wg.Go(func() {
			status("Loading skills...")
			start := time.Now()
			skills, err := skill.LoadDir(skillsDir)
			if err != nil {
				skillsErr = fmt.Errorf("engine: skills: %w", err)
				return
			}
			loadedSkills = skills
			status(fmt.Sprintf("Loaded %d skills (%s)", len(skills), time.Since(start).Round(time.Millisecond)))
		})
	}

	wg.Go(func() {
		status("Loading project context...")
		start := time.Now()
		projectRoot := filepath.Dir(dir.Root())
		loadedCtx = projectctx.Load(dir, projectRoot)
		knowledgeStale = projectctx.IsKnowledgeStale(projectRoot, dir)
		status(fmt.Sprintf("Project context ready (%s)", time.Since(start).Round(time.Millisecond)))
	})

	wg.Go(func() {
		mcpErr = e.connectMCPClients(ctx, cfg.MCPServers, status)
	})

	wg.Wait()

	// Assign after all goroutines are done to avoid data races.
	e.skills = loadedSkills
	e.projectCtx = loadedCtx
	e.knowledgeStale = knowledgeStale

	if skillsErr != nil {
		return skillsErr
	}
	if mcpErr != nil {
		return mcpErr
	}
	return nil
}
