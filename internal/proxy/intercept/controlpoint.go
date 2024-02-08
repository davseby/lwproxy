package intercept

import "sync"

// ControlPoint manages remote addresses that shouldn't be able to receive
// communication data.
type ControlPoint struct {
	mu    sync.RWMutex
	conns map[string]struct{}
}

// NewControlPoint returns a new control point.
func NewControlPoint() *ControlPoint {
	return &ControlPoint{
		conns: make(map[string]struct{}),
	}
}

// Add adds the remote address to the control point.
func (cp *ControlPoint) Add(remoteAddr string) {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	cp.conns[remoteAddr] = struct{}{}
}

// HasRemove checks whether the remote address is in the control point and
// removes it if so.
func (cp *ControlPoint) HasRemove(remoteAddr string) bool {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	_, ok := cp.conns[remoteAddr]
	delete(cp.conns, remoteAddr)

	return ok
}

// Has reports whether the remote address is in the control point.
func (cp *ControlPoint) Has(remoteAddr string) bool {
	cp.mu.RLock()
	defer cp.mu.RUnlock()

	_, ok := cp.conns[remoteAddr]
	return ok
}

// Clean cleans the control point remote addresses.
func (cp *ControlPoint) Clean() {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	cp.conns = make(map[string]struct{})
}
