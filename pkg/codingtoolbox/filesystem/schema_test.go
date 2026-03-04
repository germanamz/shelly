package filesystem_test

import (
	"path/filepath"
	"testing"

	"github.com/germanamz/shelly/pkg/codingtoolbox/filesystem"
	"github.com/germanamz/shelly/pkg/codingtoolbox/internal/schematest"
	"github.com/germanamz/shelly/pkg/codingtoolbox/permissions"
	"github.com/stretchr/testify/require"
)

func TestToolSchemas(t *testing.T) {
	store, err := permissions.New(filepath.Join(t.TempDir(), "perms.json"))
	require.NoError(t, err)

	fs := filesystem.New(store, nil, nil)
	schematest.ValidateTools(t, fs.Tools())
}
