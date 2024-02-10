package intercept

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewControlPoint(t *testing.T) {
	cp := NewControlPoint()
	require.NotNil(t, cp)
	assert.NotNil(t, cp.conns)
}

func Test_ControlPointAdd(t *testing.T) {
	cp := ControlPoint{
		conns: make(map[string]struct{}),
	}

	cp.Add("test")
	assert.Contains(t, cp.conns, "test")
}

func Test_ControlPointHasRemove(t *testing.T) {
	cp := ControlPoint{
		conns: map[string]struct{}{
			"test": {},
		},
	}

	// success
	assert.True(t, cp.HasRemove("test"))
	assert.NotContains(t, cp.conns, "test")

	// failure
	assert.False(t, cp.HasRemove("test"))
}

func Test_ControlPointClean(t *testing.T) {
	cp := ControlPoint{
		conns: map[string]struct{}{
			"test": {},
		},
	}

	cp.Clean()
	assert.Empty(t, cp.conns)
}
