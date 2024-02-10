package intercept

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewControl(t *testing.T) {
	c := NewControl()
	require.NotNil(t, c)
	assert.NotNil(t, c.conns)
}

func Test_ControlAdd(t *testing.T) {
	c := Control{
		conns: make(map[string]struct{}),
	}

	c.Add("test")
	assert.Contains(t, c.conns, "test")
}

func Test_Control_HasRemove(t *testing.T) {
	c := Control{
		conns: map[string]struct{}{
			"test": {},
		},
	}

	// success
	assert.True(t, c.HasRemove("test"))
	assert.NotContains(t, c.conns, "test")

	// failure
	assert.False(t, c.HasRemove("test"))
}

func Test_ControlClean(t *testing.T) {
	c := Control{
		conns: map[string]struct{}{
			"test": {},
		},
	}

	c.Clean()
	assert.Empty(t, c.conns)
}
