package agentctx

import "strings"

// SanitizeFilename replaces any non-alphanumeric, non-hyphen, non-underscore
// characters with hyphens so the result can be used safely as a filename
// component.
func SanitizeFilename(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	return b.String()
}
