// Package codingtoolbox provides the built-in coding tools that agents use to
// interact with the local environment. Each sub-package implements a specific
// tool category:
//
//   - [github.com/germanamz/shelly/pkg/codingtoolbox/ask] — ask_user tool for prompting the user during a session
//   - [github.com/germanamz/shelly/pkg/codingtoolbox/browser] — browser automation tools via headless Chrome (chromedp)
//   - [github.com/germanamz/shelly/pkg/codingtoolbox/filesystem] — permission-gated filesystem tools (read, write, edit, list, delete, move, copy, stat, diff, patch, mkdir)
//   - [github.com/germanamz/shelly/pkg/codingtoolbox/exec] — permission-gated command execution tool
//   - [github.com/germanamz/shelly/pkg/codingtoolbox/search] — permission-gated content and file search tools
//   - [github.com/germanamz/shelly/pkg/codingtoolbox/git] — permission-gated git operation tools (status, diff, log, commit)
//   - [github.com/germanamz/shelly/pkg/codingtoolbox/http] — permission-gated HTTP request tool with SSRF protection
//   - [github.com/germanamz/shelly/pkg/codingtoolbox/notes] — persistent note-taking tools surviving context compaction
//   - [github.com/germanamz/shelly/pkg/codingtoolbox/permissions] — shared permissions store for filesystem, exec, search, git, http, and browser
//   - [github.com/germanamz/shelly/pkg/codingtoolbox/defaults] — default toolbox builder that merges built-in toolboxes
package codingtoolbox
