package skill

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStoreAndSkills(t *testing.T) {
	st := NewStore([]Skill{
		{Name: "charlie", Content: "C"},
		{Name: "alpha", Content: "A"},
		{Name: "bravo", Content: "B"},
	}, "")

	skills := st.Skills()

	require.Len(t, skills, 3)
	assert.Equal(t, "alpha", skills[0].Name)
	assert.Equal(t, "bravo", skills[1].Name)
	assert.Equal(t, "charlie", skills[2].Name)
}

func TestStoreGetFound(t *testing.T) {
	st := NewStore([]Skill{
		{Name: "review", Content: "Review steps"},
	}, "")

	s, ok := st.Get("review")

	assert.True(t, ok)
	assert.Equal(t, "Review steps", s.Content)
}

func TestStoreGetNotFound(t *testing.T) {
	st := NewStore([]Skill{}, "")

	_, ok := st.Get("missing")

	assert.False(t, ok)
}

func TestStoreToolLoadSkillValid(t *testing.T) {
	st := NewStore([]Skill{
		{Name: "deploy", Content: "1. Build\n2. Deploy", Dir: "/project/skills/deploy"},
	}, "/project")
	tb := st.Tools()

	tools := tb.Tools()
	require.Len(t, tools, 1)
	assert.Equal(t, "load_skill", tools[0].Name)

	input, _ := json.Marshal(loadSkillInput{Name: "deploy"})
	result, err := tools[0].Handler(context.Background(), input)

	require.NoError(t, err)
	expected := "1. Build\n2. Deploy\n\n---\nSkill directory: skills/deploy\nUse filesystem tools to access supplementary files in this directory."
	assert.Equal(t, expected, result)
}

func TestStoreToolLoadSkillValidNoWorkDir(t *testing.T) {
	st := NewStore([]Skill{
		{Name: "deploy", Content: "1. Build\n2. Deploy", Dir: "/skills/deploy"},
	}, "")
	tb := st.Tools()
	tools := tb.Tools()

	input, _ := json.Marshal(loadSkillInput{Name: "deploy"})
	result, err := tools[0].Handler(context.Background(), input)

	require.NoError(t, err)
	expected := "1. Build\n2. Deploy\n\n---\nSkill directory: /skills/deploy\nUse filesystem tools to access supplementary files in this directory."
	assert.Equal(t, expected, result)
}

func TestStoreToolLoadSkillNoDirFooter(t *testing.T) {
	st := NewStore([]Skill{
		{Name: "simple", Content: "Simple content"},
	}, "")
	tb := st.Tools()
	tools := tb.Tools()

	input, _ := json.Marshal(loadSkillInput{Name: "simple"})
	result, err := tools[0].Handler(context.Background(), input)

	require.NoError(t, err)
	assert.Equal(t, "Simple content", result)
	assert.NotContains(t, result, "Skill directory:")
}

func TestStoreToolLoadSkillNotFound(t *testing.T) {
	st := NewStore([]Skill{}, "")
	tb := st.Tools()
	tools := tb.Tools()

	input, _ := json.Marshal(loadSkillInput{Name: "missing"})
	_, err := tools[0].Handler(context.Background(), input)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "skill not found: missing")
}

func TestStoreToolLoadSkillInvalidJSON(t *testing.T) {
	st := NewStore([]Skill{}, "")
	tb := st.Tools()
	tools := tb.Tools()

	_, err := tools[0].Handler(context.Background(), json.RawMessage(`not json`))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid input")
}
