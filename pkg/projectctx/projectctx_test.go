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
			ctx:  Context{External: "external", Curated: "curated", Generated: "generated"},
			want: "external\n\ncurated\n\ngenerated",
		},
		{
			name: "external and curated",
			ctx:  Context{External: "external", Curated: "curated"},
			want: "external\n\ncurated",
		},
		{
			name: "curated and generated",
			ctx:  Context{Curated: "curated content", Generated: "generated content"},
			want: "curated content\n\ngenerated content",
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
			name: "generated only",
			ctx:  Context{Generated: "generated content"},
			want: "generated content",
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

	// Create generated cache.
	localDir := filepath.Join(tmp, "local")
	require.NoError(t, os.MkdirAll(localDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(localDir, "context-cache.json"), []byte("Generated index"), 0o600))

	d := shellydir.New(tmp)
	ctx := Load(d, filepath.Dir(tmp))

	assert.Equal(t, "Project info", ctx.Curated)
	assert.Equal(t, "Generated index", ctx.Generated)
}

func TestLoad_MissingDir(t *testing.T) {
	d := shellydir.New("/nonexistent/.shelly")
	ctx := Load(d, "/nonexistent")

	assert.Empty(t, ctx.Curated)
	assert.Empty(t, ctx.Generated)
}

func TestIsStale_NoCacheFile(t *testing.T) {
	tmp := t.TempDir()
	d := shellydir.New(filepath.Join(tmp, ".shelly"))

	assert.True(t, IsStale(tmp, d))
}

func TestIsStale_NoGoMod(t *testing.T) {
	tmp := t.TempDir()
	shellyRoot := filepath.Join(tmp, ".shelly")
	require.NoError(t, os.MkdirAll(filepath.Join(shellyRoot, "local"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(shellyRoot, "local", "context-cache.json"), []byte("cached"), 0o600))

	d := shellydir.New(shellyRoot)
	// No go.mod → not stale (can't determine).
	assert.False(t, IsStale(tmp, d))
}
