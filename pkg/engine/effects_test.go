package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParamFloat_Default(t *testing.T) {
	v, err := paramFloat(nil, "threshold", 0.8)
	require.NoError(t, err)
	assert.InDelta(t, 0.8, v, 1e-9)
}

func TestParamFloat_Float64(t *testing.T) {
	v, err := paramFloat(map[string]any{"threshold": 0.5}, "threshold", 0.8)
	require.NoError(t, err)
	assert.InDelta(t, 0.5, v, 1e-9)
}

func TestParamFloat_Int(t *testing.T) {
	v, err := paramFloat(map[string]any{"threshold": 1}, "threshold", 0.8)
	require.NoError(t, err)
	assert.InDelta(t, 1.0, v, 1e-9)
}

func TestParamFloat_InvalidType(t *testing.T) {
	_, err := paramFloat(map[string]any{"threshold": "bad"}, "threshold", 0.8)
	assert.ErrorContains(t, err, "threshold must be a number")
}

func TestParamInt_Default(t *testing.T) {
	v, err := paramInt(nil, "window_size", 10)
	require.NoError(t, err)
	assert.Equal(t, 10, v)
}

func TestParamInt_Int(t *testing.T) {
	v, err := paramInt(map[string]any{"window_size": 5}, "window_size", 10)
	require.NoError(t, err)
	assert.Equal(t, 5, v)
}

func TestParamInt_Float64(t *testing.T) {
	v, err := paramInt(map[string]any{"window_size": float64(7)}, "window_size", 10)
	require.NoError(t, err)
	assert.Equal(t, 7, v)
}

func TestParamInt_InvalidType(t *testing.T) {
	_, err := paramInt(map[string]any{"window_size": true}, "window_size", 10)
	assert.ErrorContains(t, err, "window_size must be a number")
}

func TestParamStringSlice_Absent(t *testing.T) {
	v, err := paramStringSlice(nil, "exclude")
	require.NoError(t, err)
	assert.Nil(t, v)
}

func TestParamStringSlice_Valid(t *testing.T) {
	params := map[string]any{"exclude": []any{"a", "b", "c"}}
	v, err := paramStringSlice(params, "exclude")
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c"}, v)
}

func TestParamStringSlice_NotArray(t *testing.T) {
	_, err := paramStringSlice(map[string]any{"exclude": "bad"}, "exclude")
	assert.ErrorContains(t, err, "exclude must be an array")
}

func TestParamStringSlice_NonStringItem(t *testing.T) {
	_, err := paramStringSlice(map[string]any{"exclude": []any{"a", 42}}, "exclude")
	assert.ErrorContains(t, err, "exclude items must be strings")
}
