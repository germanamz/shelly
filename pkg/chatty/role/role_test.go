package role

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRole_Valid(t *testing.T) {
	tests := []struct {
		role Role
		want bool
	}{
		{System, true},
		{User, true},
		{Assistant, true},
		{Tool, true},
		{Role("unknown"), false},
		{Role(""), false},
	}

	for _, tt := range tests {
		t.Run(tt.role.String(), func(t *testing.T) {
			assert.Equal(t, tt.want, tt.role.Valid())
		})
	}
}

func TestRole_String(t *testing.T) {
	assert.Equal(t, "system", System.String())
	assert.Equal(t, "user", User.String())
	assert.Equal(t, "assistant", Assistant.String())
	assert.Equal(t, "tool", Tool.String())
}
