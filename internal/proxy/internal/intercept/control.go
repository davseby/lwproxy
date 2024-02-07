package intercept

import (
	"sync"
)

// Control manages remote addresses that shouldn't be able to receive
// communication data.
type Control struct {
	mu    sync.RWMutex
	conns map[string]struct{}
}

// NewControl returns a new control.
func NewControl() *Control {
	return &Control{
		conns: make(map[string]struct{}),
	}
}

// Add adds the remote address to the control.
func (c *Control) Add(remoteAddr string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.conns[remoteAddr] = struct{}{}
}

// HasRemove checks whether the remote address is in the control and
// removes it if so.
func (c *Control) HasRemove(remoteAddr string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	_, ok := c.conns[remoteAddr]
	delete(c.conns, remoteAddr)

	return ok
}

// Clean cleans the control remote addresses.
func (c *Control) Clean() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.conns = make(map[string]struct{})
}
