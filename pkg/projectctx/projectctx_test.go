package projectctx

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/germanamz/shelly/pkg/shellydir"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContext_String(t *testing.T) {
	tests := []struct {
		name string
		ctx  Context
		want string
	}{
		{
			name: "all fields",
			ctx:  Context{External: "external", Curated: "curated"},
			want: "external\n\ncurated",
		},
		{
			name: "external only",
			ctx:  Context{External: "external content"},
			want: "external content",
		},
		{
			name: "curated only",
			ctx:  Context{Curated: "curated content"},
			want: "curated content",
		},
		{
			name: "empty",
			ctx:  Context{},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.ctx.String())
		})
	}
}

func TestLoadCurated(t *testing.T) {
	tmp := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(tmp, "context.md"), []byte("# Context\n\nProject instructions."), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "notes.md"), []byte("Additional notes."), 0o600))
	// Non-md files should be ignored.
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "config.yaml"), []byte("yaml: true"), 0o600))

	d := shellydir.New(tmp)
	result := LoadCurated(d)

	assert.Contains(t, result, "# Context")
	assert.Contains(t, result, "Project instructions.")
	assert.Contains(t, result, "Additional notes.")
	assert.NotContains(t, result, "yaml")
}

func TestLoadCurated_NoFiles(t *testing.T) {
	tmp := t.TempDir()
	d := shellydir.New(tmp)

	assert.Empty(t, LoadCurated(d))
}

func TestLoadCurated_EmptyFiles(t *testing.T) {
	tmp := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(tmp, "empty.md"), []byte("  \n\n  "), 0o600))

	d := shellydir.New(tmp)
	assert.Empty(t, LoadCurated(d))
}

func TestLoad(t *testing.T) {
	tmp := t.TempDir()

	// Create curated content.
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "context.md"), []byte("Project info"), 0o600))

	d := shellydir.New(tmp)
	ctx := Load(d, filepath.Dir(tmp))

	assert.Equal(t, "Project info", ctx.Curated)
}

func TestLoad_MissingDir(t *testing.T) {
	d := shellydir.New("/nonexistent/.shelly")
	ctx := Load(d, "/nonexistent")

	assert.Empty(t, ctx.Curated)
}
