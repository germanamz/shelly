// Package mcproots provides shared utilities for MCP Roots protocol support.
// It handles context plumbing and path-checking logic used by both the client
// side (sending approved directories as roots to MCP servers) and the server
// side (constraining filesystem access based on client-declared roots).
//
// This package is intentionally zero-dependency (no MCP SDK imports) so it can
// be imported by both mcpclient, mcpserver, and filesystem without introducing
// circular dependencies. SDK-dependent conversions (DirToRoot, RootPaths) live
// in their respective packages.
package mcproots

import (
	"context"
	"path/filepath"
	"strings"
)

type contextKey struct{}

// WithRoots returns a new context carrying the given root paths.
func WithRoots(ctx context.Context, roots []string) context.Context {
	return context.WithValue(ctx, contextKey{}, roots)
}

// FromContext extracts root paths from the context.
// Returns nil if no roots were set (meaning unconstrained access).
func FromContext(ctx context.Context) []string {
	roots, _ := ctx.Value(contextKey{}).([]string)
	return roots
}

// IsPathAllowed reports whether absPath falls under at least one of the given
// roots. A nil roots slice means unconstrained (always returns true). An empty
// non-nil slice means nothing is allowed (always returns false).
func IsPathAllowed(absPath string, roots []string) bool {
	if roots == nil {
		return true
	}

	for _, root := range roots {
		if pathIsUnder(absPath, root) {
			return true
		}
	}

	return false
}

// pathIsUnder reports whether child is equal to or under parent.
func pathIsUnder(child, parent string) bool {
	child = filepath.Clean(child)
	parent = filepath.Clean(parent)

	if child == parent {
		return true
	}

	// Ensure parent ends with separator so "/tmp" doesn't match "/tmpfoo".
	prefix := parent + string(filepath.Separator)

	return strings.HasPrefix(child, prefix)
}
