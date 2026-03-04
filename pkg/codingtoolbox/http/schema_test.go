package http_test

import (
	"path/filepath"
	"testing"

	shellyhttp "github.com/germanamz/shelly/pkg/codingtoolbox/http"
	"github.com/germanamz/shelly/pkg/codingtoolbox/internal/schematest"
	"github.com/germanamz/shelly/pkg/codingtoolbox/permissions"
	"github.com/stretchr/testify/require"
)

func TestToolSchemas(t *testing.T) {
	store, err := permissions.New(filepath.Join(t.TempDir(), "perms.json"))
	require.NoError(t, err)

	h := shellyhttp.New(store, nil)
	schematest.ValidateTools(t, h.Tools())
}
