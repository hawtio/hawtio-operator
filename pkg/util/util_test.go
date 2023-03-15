package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContains(t *testing.T) {
	assert.True(t, Contains([]string{"a", "b", "c"}, "b"))
	assert.False(t, Contains([]string{"a", "b", "c"}, "x"))
	assert.False(t, Contains([]string{}, "x"))
	assert.False(t, Contains(nil, "x"))
}
