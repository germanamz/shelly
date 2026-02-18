package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPlaceholder(t *testing.T) {
	got := "hello"
	assert.Equal(t, "hello", got)
}
