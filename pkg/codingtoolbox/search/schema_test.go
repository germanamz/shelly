package search_test

import (
	"path/filepath"
	"testing"

	"github.com/germanamz/shelly/pkg/codingtoolbox/internal/schematest"
	"github.com/germanamz/shelly/pkg/codingtoolbox/permissions"
	"github.com/germanamz/shelly/pkg/codingtoolbox/search"
	"github.com/stretchr/testify/require"
)

func TestToolSchemas(t *testing.T) {
	store, err := permissions.New(filepath.Join(t.TempDir(), "perms.json"))
	require.NoError(t, err)

	s := search.New(store, nil)
	schematest.ValidateTools(t, s.Tools())
}
