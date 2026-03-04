package notes_test

import (
	"testing"

	"github.com/germanamz/shelly/pkg/codingtoolbox/internal/schematest"
	"github.com/germanamz/shelly/pkg/codingtoolbox/notes"
)

func TestToolSchemas(t *testing.T) {
	s := notes.New(t.TempDir())
	schematest.ValidateTools(t, s.Tools())
}
