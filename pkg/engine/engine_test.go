package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/germanamz/shelly/pkg/agent/effects"
	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/germanamz/shelly/pkg/modeladapter"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockCompleter is a simple Completer that returns a canned reply.
type mockCompleter struct {
	reply string
}

func (m *mockCompleter) Complete(_ context.Context, _ *chat.Chat, _ []toolbox.Tool) (message.Message, error) {
	return message.NewText("bot", role.Assistant, m.reply), nil
}

func TestEngine_NewSession_DefaultAgent(t *testing.T) {
	RegisterProvider("mock", func(_ ProviderConfig) (modeladapter.Completer, error) {
		return &mockCompleter{reply: "hello"}, nil
	})

	cfg := Config{
		Providers: []ProviderConfig{{Name: "p1", Kind: "mock", Model: "test"}},
		Agents:    []AgentConfig{{Name: "bot", Description: "test bot", Provider: "p1"}},
	}

	eng, err := New(context.Background(), cfg)
	require.NoError(t, err)
	defer func() { _ = eng.Close() }()

	sess, err := eng.NewSession("")
	require.NoError(t, err)
	assert.NotEmpty(t, sess.ID())

	found, ok := eng.Session(sess.ID())
	assert.True(t, ok)
	assert.Equal(t, sess, found)
}

func TestEngine_NewSession_NamedAgent(t *testing.T) {
	RegisterProvider("mock", func(_ ProviderConfig) (modeladapter.Completer, error) {
		return &mockCompleter{reply: "hi"}, nil
	})

	cfg := Config{
		Providers: []ProviderConfig{{Name: "p1", Kind: "mock"}},
		Agents: []AgentConfig{
			{Name: "alpha", Provider: "p1"},
			{Name: "beta", Provider: "p1"},
		},
	}

	eng, err := New(context.Background(), cfg)
	require.NoError(t, err)
	defer func() { _ = eng.Close() }()

	_, err = eng.NewSession("beta")
	assert.NoError(t, err)
}

func TestEngine_NewSession_NotFound(t *testing.T) {
	RegisterProvider("mock", func(_ ProviderConfig) (modeladapter.Completer, error) {
		return &mockCompleter{reply: "x"}, nil
	})

	cfg := Config{
		Providers: []ProviderConfig{{Name: "p1", Kind: "mock"}},
		Agents:    []AgentConfig{{Name: "a1", Provider: "p1"}},
	}

	eng, err := New(context.Background(), cfg)
	require.NoError(t, err)
	defer func() { _ = eng.Close() }()

	_, err = eng.NewSession("nope")
	assert.Error(t, err)
}

func TestEngine_SendAndEvents(t *testing.T) {
	RegisterProvider("mock", func(_ ProviderConfig) (modeladapter.Completer, error) {
		return &mockCompleter{reply: "world"}, nil
	})

	cfg := Config{
		Providers: []ProviderConfig{{Name: "p1", Kind: "mock"}},
		Agents:    []AgentConfig{{Name: "bot", Provider: "p1"}},
	}

	eng, err := New(context.Background(), cfg)
	require.NoError(t, err)
	defer func() { _ = eng.Close() }()

	sub := eng.Events().Subscribe(32)
	defer eng.Events().Unsubscribe(sub)

	sess, err := eng.NewSession("")
	require.NoError(t, err)

	reply, err := sess.Send(context.Background(), "hello")
	require.NoError(t, err)
	assert.Equal(t, "world", reply.TextContent())

	// Drain events — expect at least AgentStart and AgentEnd.
	var kinds []EventKind
	timeout := time.After(time.Second)
	for {
		select {
		case e := <-sub.C:
			kinds = append(kinds, e.Kind)
		case <-timeout:
			goto done
		}
	}
done:
	assert.Contains(t, kinds, EventAgentStart)
	assert.Contains(t, kinds, EventAgentEnd)
}

func TestEngine_StateEnabled(t *testing.T) {
	RegisterProvider("mock", func(_ ProviderConfig) (modeladapter.Completer, error) {
		return &mockCompleter{reply: "ok"}, nil
	})

	cfg := Config{
		Providers: []ProviderConfig{{Name: "p1", Kind: "mock"}},
		Agents:    []AgentConfig{{Name: "a1", Provider: "p1", Toolboxes: []ToolboxRef{{Name: "state"}}}},
	}

	eng, err := New(context.Background(), cfg)
	require.NoError(t, err)
	defer func() { _ = eng.Close() }()

	assert.NotNil(t, eng.State())
}

func TestEngine_TasksEnabled(t *testing.T) {
	RegisterProvider("mock", func(_ ProviderConfig) (modeladapter.Completer, error) {
		return &mockCompleter{reply: "ok"}, nil
	})

	cfg := Config{
		Providers: []ProviderConfig{{Name: "p1", Kind: "mock"}},
		Agents:    []AgentConfig{{Name: "a1", Provider: "p1", Toolboxes: []ToolboxRef{{Name: "tasks"}}}},
	}

	eng, err := New(context.Background(), cfg)
	require.NoError(t, err)
	defer func() { _ = eng.Close() }()

	assert.NotNil(t, eng.Tasks())
}

func TestEngine_TasksDisabled(t *testing.T) {
	RegisterProvider("mock", func(_ ProviderConfig) (modeladapter.Completer, error) {
		return &mockCompleter{reply: "ok"}, nil
	})

	cfg := Config{
		Providers: []ProviderConfig{{Name: "p1", Kind: "mock"}},
		Agents:    []AgentConfig{{Name: "a1", Provider: "p1"}},
	}

	eng, err := New(context.Background(), cfg)
	require.NoError(t, err)
	defer func() { _ = eng.Close() }()

	assert.Nil(t, eng.Tasks())
}

func TestEngine_RemoveSession(t *testing.T) {
	RegisterProvider("mock", func(_ ProviderConfig) (modeladapter.Completer, error) {
		return &mockCompleter{reply: "ok"}, nil
	})

	cfg := Config{
		Providers: []ProviderConfig{{Name: "p1", Kind: "mock"}},
		Agents:    []AgentConfig{{Name: "bot", Provider: "p1"}},
	}

	eng, err := New(context.Background(), cfg)
	require.NoError(t, err)
	defer func() { _ = eng.Close() }()

	sess, err := eng.NewSession("")
	require.NoError(t, err)

	// Session exists.
	_, ok := eng.Session(sess.ID())
	assert.True(t, ok)

	// Remove it.
	assert.True(t, eng.RemoveSession(sess.ID()))

	// Session no longer exists.
	_, ok = eng.Session(sess.ID())
	assert.False(t, ok)

	// Removing again returns false.
	assert.False(t, eng.RemoveSession(sess.ID()))
}

func TestEngine_InvalidConfig(t *testing.T) {
	_, err := New(context.Background(), Config{})
	assert.Error(t, err)
}

func TestEngine_SkillsLoadError(t *testing.T) {
	RegisterProvider("mock", func(_ ProviderConfig) (modeladapter.Completer, error) {
		return &mockCompleter{reply: "ok"}, nil
	})

	// Create a .shelly dir with a skills dir containing an invalid skill.
	dir := t.TempDir()
	shellyDir := dir + "/.shelly"
	skillsDir := shellyDir + "/skills/bad-skill"
	require.NoError(t, os.MkdirAll(skillsDir, 0o750))
	// Write a SKILL.md with invalid frontmatter to trigger a load error.
	require.NoError(t, os.WriteFile(skillsDir+"/SKILL.md", []byte("---\ninvalid: [broken\n---\n"), 0o600))

	cfg := Config{
		ShellyDir: shellyDir,
		Providers: []ProviderConfig{{Name: "p1", Kind: "mock"}},
		Agents:    []AgentConfig{{Name: "a1", Provider: "p1"}},
	}

	_, err := New(context.Background(), cfg)
	assert.ErrorContains(t, err, "engine: skills:")
}

func TestEngine_SkillsDirMissing(t *testing.T) {
	RegisterProvider("mock", func(_ ProviderConfig) (modeladapter.Completer, error) {
		return &mockCompleter{reply: "ok"}, nil
	})

	// .shelly dir that does not exist at all - no error expected.
	cfg := Config{
		ShellyDir: "/tmp/nonexistent-shelly-dir-" + t.Name(),
		Providers: []ProviderConfig{{Name: "p1", Kind: "mock"}},
		Agents:    []AgentConfig{{Name: "a1", Provider: "p1"}},
	}

	eng, err := New(context.Background(), cfg)
	require.NoError(t, err)
	defer func() { _ = eng.Close() }()
}

func TestSession_SendParts(t *testing.T) {
	RegisterProvider("mock", func(_ ProviderConfig) (modeladapter.Completer, error) {
		return &mockCompleter{reply: "ok"}, nil
	})

	cfg := Config{
		Providers: []ProviderConfig{{Name: "p1", Kind: "mock"}},
		Agents:    []AgentConfig{{Name: "bot", Provider: "p1"}},
	}

	eng, err := New(context.Background(), cfg)
	require.NoError(t, err)
	defer func() { _ = eng.Close() }()

	sess, err := eng.NewSession("")
	require.NoError(t, err)

	reply, err := sess.SendParts(context.Background(), content.Text{Text: "hi"})
	require.NoError(t, err)
	assert.Equal(t, "ok", reply.TextContent())
}

func TestSession_ConcurrentSendBlocked(t *testing.T) {
	RegisterProvider("mock", func(_ ProviderConfig) (modeladapter.Completer, error) {
		return &mockCompleter{reply: "ok"}, nil
	})

	cfg := Config{
		Providers: []ProviderConfig{{Name: "p1", Kind: "mock"}},
		Agents:    []AgentConfig{{Name: "bot", Provider: "p1"}},
	}

	eng, err := New(context.Background(), cfg)
	require.NoError(t, err)
	defer func() { _ = eng.Close() }()

	sess, err := eng.NewSession("")
	require.NoError(t, err)

	// Manually lock the session.
	sess.mu.Lock()
	sess.active = true
	sess.mu.Unlock()

	_, err = sess.Send(context.Background(), "test")
	require.ErrorContains(t, err, "already active")

	// Unlock for cleanup.
	sess.mu.Lock()
	sess.active = false
	sess.mu.Unlock()
}

func TestEngine_LoadsProjectContext(t *testing.T) {
	RegisterProvider("mock", func(_ ProviderConfig) (modeladapter.Completer, error) {
		return &mockCompleter{reply: "ok"}, nil
	})

	projectDir := t.TempDir()

	shellyPath := filepath.Join(projectDir, ".shelly")
	require.NoError(t, os.MkdirAll(shellyPath, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(shellyPath, "context.md"), []byte("# My Project"), 0o600))

	cfg := Config{
		ShellyDir: shellyPath,
		Providers: []ProviderConfig{{Name: "p1", Kind: "mock"}},
		Agents:    []AgentConfig{{Name: "a1", Provider: "p1"}},
	}

	eng, err := New(context.Background(), cfg)
	require.NoError(t, err)
	defer func() { _ = eng.Close() }()

	assert.Contains(t, eng.projectCtx.Curated, "# My Project")
}

func TestDefaultPipelineIncludesObservationMask(t *testing.T) {
	// When no explicit effects are configured the auto-generated default
	// pipeline should include observation_mask between trim_tool_results
	// and compact.
	defaultConfigs := []EffectConfig{
		{Kind: "trim_tool_results"},
		{Kind: "observation_mask", Params: map[string]any{"threshold": 0.5}},
		{Kind: "compact", Params: map[string]any{"threshold": 0.8}},
	}

	wctx := EffectWiringContext{
		ContextWindow: 200000,
		AgentName:     "test",
	}

	effs, err := buildEffects(defaultConfigs, wctx)
	require.NoError(t, err)
	require.Len(t, effs, 3)

	// Verify that an ObservationMaskEffect is present among the built effects.
	var hasObsMask bool
	for _, e := range effs {
		if _, ok := e.(*effects.ObservationMaskEffect); ok {
			hasObsMask = true
			break
		}
	}
	assert.True(t, hasObsMask, "default pipeline should include ObservationMaskEffect")
}

func TestEffectPriority_ObservationMask(t *testing.T) {
	eff := effects.NewObservationMaskEffect(effects.ObservationMaskConfig{
		ContextWindow: 200000,
		Threshold:     0.5,
	})
	assert.Equal(t, 0, effectPriority(eff), "ObservationMaskEffect should have compaction-class priority (0)")
}

func TestSession_AutoSave(t *testing.T) {
	RegisterProvider("mock", func(_ ProviderConfig) (modeladapter.Completer, error) {
		return &mockCompleter{reply: "saved"}, nil
	})

	sessDir := filepath.Join(t.TempDir(), ".shelly")
	require.NoError(t, os.MkdirAll(sessDir, 0o750))

	cfg := Config{
		ShellyDir: sessDir,
		Providers: []ProviderConfig{{Name: "p1", Kind: "mock", Model: "test-model"}},
		Agents:    []AgentConfig{{Name: "bot", Description: "test bot", Provider: "p1"}},
	}

	eng, err := New(context.Background(), cfg)
	require.NoError(t, err)
	defer func() { _ = eng.Close() }()

	sess, err := eng.NewSession("")
	require.NoError(t, err)

	_, err = sess.Send(context.Background(), "hello")
	require.NoError(t, err)

	// Session should have been auto-saved.
	listed, err := eng.SessionStore().List()
	require.NoError(t, err)
	require.Len(t, listed, 1)
	assert.Equal(t, sess.PersistID(), listed[0].ID)
	assert.Equal(t, "bot", listed[0].Agent)
	assert.Equal(t, "hello", listed[0].Preview)
	assert.Equal(t, "mock", listed[0].Provider.Kind)
	assert.Equal(t, "test-model", listed[0].Provider.Model)
}

func TestEngine_ResumeSession(t *testing.T) {
	RegisterProvider("mock", func(_ ProviderConfig) (modeladapter.Completer, error) {
		return &mockCompleter{reply: "resumed"}, nil
	})

	sessDir := filepath.Join(t.TempDir(), ".shelly")
	require.NoError(t, os.MkdirAll(sessDir, 0o750))

	cfg := Config{
		ShellyDir: sessDir,
		Providers: []ProviderConfig{{Name: "p1", Kind: "mock", Model: "m1"}},
		Agents:    []AgentConfig{{Name: "bot", Description: "test bot", Provider: "p1"}},
	}

	eng, err := New(context.Background(), cfg)
	require.NoError(t, err)
	defer func() { _ = eng.Close() }()

	// Create and send to get a persisted session.
	sess, err := eng.NewSession("")
	require.NoError(t, err)
	_, err = sess.Send(context.Background(), "first message")
	require.NoError(t, err)
	persistID := sess.PersistID()
	origCreatedAt := sess.CreatedAt()

	// Resume the session.
	resumed, err := eng.ResumeSession(persistID)
	require.NoError(t, err)
	assert.Equal(t, persistID, resumed.PersistID())
	assert.Equal(t, origCreatedAt.Unix(), resumed.CreatedAt().Unix())
	assert.NotEqual(t, sess.ID(), resumed.ID()) // Engine ID is different.

	// The resumed session should have messages from the original.
	// System prompt + user "first message" + assistant reply = at least 3.
	assert.GreaterOrEqual(t, resumed.Chat().Len(), 3)

	// Sending on resumed session should work and auto-save.
	_, err = resumed.Send(context.Background(), "second message")
	require.NoError(t, err)

	// Verify save overwrites the same file (only 1 session in store).
	listed, err := eng.SessionStore().List()
	require.NoError(t, err)
	assert.Len(t, listed, 1)
	assert.Equal(t, persistID, listed[0].ID)
	assert.Equal(t, "second message", listed[0].Preview)
}

func TestEngine_ResumeSession_NotFound(t *testing.T) {
	RegisterProvider("mock", func(_ ProviderConfig) (modeladapter.Completer, error) {
		return &mockCompleter{reply: "ok"}, nil
	})

	cfg := Config{
		Providers: []ProviderConfig{{Name: "p1", Kind: "mock"}},
		Agents:    []AgentConfig{{Name: "bot", Provider: "p1"}},
	}

	eng, err := New(context.Background(), cfg)
	require.NoError(t, err)
	defer func() { _ = eng.Close() }()

	_, err = eng.ResumeSession("nonexistent")
	assert.Error(t, err)
}
