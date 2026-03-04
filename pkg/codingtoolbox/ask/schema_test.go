package ask_test

import (
	"testing"

	"github.com/germanamz/shelly/pkg/codingtoolbox/ask"
	"github.com/germanamz/shelly/pkg/codingtoolbox/internal/schematest"
)

func TestToolSchemas(t *testing.T) {
	r := ask.NewResponder(nil)
	schematest.ValidateTools(t, r.Tools())
}
