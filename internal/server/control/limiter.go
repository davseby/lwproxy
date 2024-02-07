package control

import (
	"sync"

	"golang.org/x/exp/slog"
)

// Limiter is a struct that holds the bandwidth limiter configuratio.
type Limiter struct {
	log *slog.Logger

	bytes struct {
		mu   sync.RWMutex
		max  int64
		used int64
	}
}

// NewLimiter creates a new limiter.
func NewLimiter(log *slog.Logger, maxBytes int64) *Limiter {
	l := &Limiter{
		log: log.With("thread", "limiter"),
	}

	l.bytes.max = maxBytes

	return l
}

// UseBytes uses the given amount of bytes and returns true if the limiter
// hasn't reached a limit.
func (l *Limiter) UseBytes(bytes int64) bool {
	l.bytes.mu.Lock()
	defer l.bytes.mu.Unlock()

	if !l.hasAvailableBytes(bytes + l.bytes.used) {
		return false
	}

	l.bytes.used += bytes

	l.log.With("used", l.bytes.used).
		With("max", l.bytes.max).
		Debug("used bytes")

	return true
}

// hasAvailableBytes returns true if the limiter hasn't reached a limit.
// NOTE: Concurrently unsafe method.
func (l *Limiter) hasAvailableBytes(total int64) bool {
	return total < l.bytes.max
}
