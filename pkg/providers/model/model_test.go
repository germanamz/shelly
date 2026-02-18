package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestModel_ZeroValue(t *testing.T) {
	var m Model

	assert.Empty(t, m.Name)
	assert.Zero(t, m.Temperature)
	assert.Zero(t, m.MaxTokens)
}

func TestModel_StructLiteral(t *testing.T) {
	m := Model{
		Name:        "gpt-4o",
		Temperature: 0.7,
		MaxTokens:   1024,
	}

	assert.Equal(t, "gpt-4o", m.Name)
	assert.InDelta(t, 0.7, m.Temperature, 1e-9)
	assert.Equal(t, 1024, m.MaxTokens)
}

func TestModel_Embedding(t *testing.T) {
	type Config struct {
		Model
		BaseURL string
	}

	cfg := Config{
		Model:   Model{Name: "claude-sonnet-4-20250514"},
		BaseURL: "https://api.example.com",
	}

	assert.Equal(t, "claude-sonnet-4-20250514", cfg.Name)
	assert.Equal(t, "https://api.example.com", cfg.BaseURL)
}
