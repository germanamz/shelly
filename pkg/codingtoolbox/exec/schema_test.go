package exec_test

import (
	"path/filepath"
	"testing"

	"github.com/germanamz/shelly/pkg/codingtoolbox/exec"
	"github.com/germanamz/shelly/pkg/codingtoolbox/internal/schematest"
	"github.com/germanamz/shelly/pkg/codingtoolbox/permissions"
	"github.com/stretchr/testify/require"
)

func TestToolSchemas(t *testing.T) {
	store, err := permissions.New(filepath.Join(t.TempDir(), "perms.json"))
	require.NoError(t, err)

	e := exec.New(store, nil)
	schematest.ValidateTools(t, e.Tools())
}
