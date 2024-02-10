// package memory implements an in memory database for the proxy service.
package memory

import (
	"sync/atomic"
)

// DB is an in memory database.
type DB struct {
	bytes *atomic.Int64
}

// NewDB creates a new in memory database.
func NewDB() *DB {
	return &DB{
		bytes: &atomic.Int64{},
	}
}
