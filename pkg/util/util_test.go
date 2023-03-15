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

func TestMatch(t *testing.T) {
	assert.True(t, Match("hawt.io/label1", "hawt.io/label1"))
	assert.True(t, Match("hawt.io/*", "hawt.io/label1"))
	assert.True(t, Match("^\\s\\n.?+(abc)[def]{ghi}|\\$", "^\\s\\n.?+(abc)[def]{ghi}|\\$"))
	assert.True(t, Match("", ""))
	assert.False(t, Match("a", "b"))
	assert.False(t, Match("hawt.io/label1", "hawt_io/label1"))
	assert.False(t, Match("", "hawt.io/label1"))
}
