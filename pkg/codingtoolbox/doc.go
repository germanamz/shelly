// Package codingtoolbox provides the built-in coding tools that agents use to
// interact with the local environment. Each sub-package implements a specific
// tool category:
//
//   - [github.com/germanamz/shelly/pkg/codingtoolbox/ask] — ask_user tool for prompting the user during a session
//   - [github.com/germanamz/shelly/pkg/codingtoolbox/filesystem] — permission-gated filesystem tools (read, write, edit, list)
//   - [github.com/germanamz/shelly/pkg/codingtoolbox/exec] — permission-gated command execution tool
//   - [github.com/germanamz/shelly/pkg/codingtoolbox/permissions] — shared permissions store for filesystem and exec
//   - [github.com/germanamz/shelly/pkg/codingtoolbox/defaults] — default toolbox builder that merges built-in toolboxes
package codingtoolbox
