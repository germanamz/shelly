package engine

import (
	"context"
	"fmt"
	"os"
	"sort"

	"github.com/germanamz/shelly/pkg/codingtoolbox/ask"
	shellyexec "github.com/germanamz/shelly/pkg/codingtoolbox/exec"
	"github.com/germanamz/shelly/pkg/codingtoolbox/filesystem"
	shellygit "github.com/germanamz/shelly/pkg/codingtoolbox/git"
	shellyhttp "github.com/germanamz/shelly/pkg/codingtoolbox/http"
	"github.com/germanamz/shelly/pkg/codingtoolbox/notes"
	"github.com/germanamz/shelly/pkg/codingtoolbox/permissions"
	"github.com/germanamz/shelly/pkg/codingtoolbox/search"
	"github.com/germanamz/shelly/pkg/shellydir"
	"github.com/germanamz/shelly/pkg/state"
	"github.com/germanamz/shelly/pkg/tasks"
)

// builtinToolboxNames are toolbox names managed by the engine itself (not MCP).
var builtinToolboxNames = map[string]struct{}{
	"state":      {},
	"tasks":      {},
	"ask":        {},
	"filesystem": {},
	"exec":       {},
	"search":     {},
	"git":        {},
	"http":       {},
	"notes":      {},
}

// BuiltinToolboxNames returns the sorted list of built-in toolbox names.
func BuiltinToolboxNames() []string {
	names := make([]string, 0, len(builtinToolboxNames))
	for name := range builtinToolboxNames {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// referencedBuiltins returns the set of built-in toolbox names that appear in
// at least one agent's Toolboxes list.
func referencedBuiltins(agents []AgentConfig) map[string]struct{} {
	refs := make(map[string]struct{})
	for _, a := range agents {
		for _, ref := range a.Toolboxes {
			if _, ok := builtinToolboxNames[ref.Name]; ok {
				refs[ref.Name] = struct{}{}
			}
		}
	}
	return refs
}

// wireBuiltinToolboxes creates all built-in toolboxes referenced by agents and
// populates e.toolboxes. Called once from New().
func (e *Engine) wireBuiltinToolboxes(cfg Config, dir shellydir.Dir) error {
	e.wireResponder()

	refs := referencedBuiltins(cfg.Agents)

	e.wireStores(refs)
	e.wireNotes(refs, dir)

	return e.wirePermissionGatedTools(cfg, dir, refs)
}

// wireResponder creates the ask responder toolbox (always available).
func (e *Engine) wireResponder() {
	e.responder = ask.NewResponder(func(ctx context.Context, q ask.Question) {
		publishFromContext(e.events, ctx, EventAskUser, q)
	})
	e.toolboxes["ask"] = e.responder.Tools()
}

// wireStores creates state and task stores if referenced by any agent.
func (e *Engine) wireStores(refs map[string]struct{}) {
	if _, ok := refs["state"]; ok {
		e.store = &state.Store{}
		e.toolboxes["state"] = e.store.Tools("shared")
	}

	if _, ok := refs["tasks"]; ok {
		e.taskStore = &tasks.Store{}
		e.toolboxes["tasks"] = e.taskStore.Tools("shared")
	}
}

// wireNotes creates the notes store if referenced. Notes persist in .shelly/notes/.
func (e *Engine) wireNotes(refs map[string]struct{}, dir shellydir.Dir) {
	if _, ok := refs["notes"]; ok {
		notesDir := dir.NotesDir()
		notesStore := notes.New(notesDir)
		e.toolboxes["notes"] = notesStore.Tools()
	}
}

// wirePermissionGatedTools creates filesystem, exec, search, git, and http
// toolboxes if referenced. All share a single permissions store.
func (e *Engine) wirePermissionGatedTools(cfg Config, dir shellydir.Dir, refs map[string]struct{}) error {
	permToolboxes := []string{"filesystem", "exec", "search", "git", "http"}
	needsPerm := false
	for _, name := range permToolboxes {
		if _, ok := refs[name]; ok {
			needsPerm = true
			break
		}
	}
	if !needsPerm {
		return nil
	}

	// Resolve permissions file path: prefer shellydir, fall back to config.
	permFile := dir.PermissionsPath()
	if !dir.Exists() && cfg.Filesystem.PermissionsFile != "" {
		permFile = cfg.Filesystem.PermissionsFile
	}

	permStore, err := permissions.New(permFile)
	if err != nil {
		return fmt.Errorf("engine: permissions: %w", err)
	}

	// Pre-approve the process CWD so sub-agents inherit filesystem
	// access to the working directory without being prompted.
	if cwd, cwdErr := os.Getwd(); cwdErr == nil {
		_ = permStore.ApproveDir(cwd)
	}

	if _, ok := refs["filesystem"]; ok {
		notifyFn := func(ctx context.Context, message string) {
			publishFromContext(e.events, ctx, EventFileChange, message)
		}
		fsTools := filesystem.New(permStore, e.responder.Ask, notifyFn)
		e.toolboxes["filesystem"] = fsTools.Tools()
	}

	if _, ok := refs["exec"]; ok {
		execTools := shellyexec.New(permStore, e.responder.Ask)
		e.toolboxes["exec"] = execTools.Tools()
	}

	if _, ok := refs["search"]; ok {
		searchTools := search.New(permStore, e.responder.Ask)
		e.toolboxes["search"] = searchTools.Tools()
	}

	if _, ok := refs["git"]; ok {
		gitTools := shellygit.New(permStore, e.responder.Ask, cfg.Git.WorkDir)
		e.toolboxes["git"] = gitTools.Tools()
	}

	if _, ok := refs["http"]; ok {
		httpTools := shellyhttp.New(permStore, e.responder.Ask)
		e.toolboxes["http"] = httpTools.Tools()
	}

	// Seed MCP clients with currently-approved directories as roots,
	// and dynamically propagate new approvals.
	e.wireRoots(permStore)

	return nil
}

// taskBoardAdapter implements agent.TaskBoard using a *tasks.Store.
type taskBoardAdapter struct {
	store *tasks.Store
}

func (a *taskBoardAdapter) ClaimTask(id, agentName string) error {
	return a.store.Reassign(id, agentName)
}

func (a *taskBoardAdapter) UpdateTaskStatus(id, status string) error {
	s := tasks.Status(status)
	return a.store.Update(id, tasks.Update{Status: &s})
}
