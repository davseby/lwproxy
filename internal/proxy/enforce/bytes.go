package enforce

import (
	"context"
	"errors"
	"sync"

	"golang.org/x/exp/slog"
)

// ErrLimitExceeded is an error for when the bytes limit is exceeded.
var ErrLimitExceeded = errors.New("bytes limit exceeded")

// BytesLimiter is a struct that supervises bytes usage and limits it.
type BytesLimiter struct {
	log *slog.Logger

	mu sync.RWMutex
	db DB

	maxBytes int64
}

// NewBytesLimiter creates a new limiter.
func NewBytesLimiter(log *slog.Logger, db DB, maxBytes int64) *BytesLimiter {
	return &BytesLimiter{
		log:      log.With("job", "limiter"),
		db:       db,
		maxBytes: maxBytes,
	}
}

// CheckBytes checks the amount of bytes used and returns an error if the
// limit is exceeded.
func (bl *BytesLimiter) CheckBytes() (bool, error) {
	bl.mu.RLock()
	defer bl.mu.RUnlock()

	bytes, err := bl.db.FetchBytes(context.Background())
	if err != nil {
		return false, err
	}

	return bytes < bl.maxBytes, nil
}

// UseBytes uses the given amount of bytes and returns true if the limiter
// hasn't reached a limit.
func (bl *BytesLimiter) UseBytes(usedBytes int64) error {
	bl.mu.Lock()
	defer bl.mu.Unlock()

	bytes, err := bl.db.FetchBytes(context.Background())
	if err != nil {
		return err
	}

	var overflow bool

	if bytes+usedBytes > bl.maxBytes {
		overflow = true
	}

	if err := bl.db.IncreaseBytes(context.Background(), usedBytes); err != nil {
		return err
	}

	if overflow {
		return ErrLimitExceeded
	}

	return nil
}

// NoopBytesLimiter is a no-op limiter.
type NoopBytesLimiter struct{}

// NewNoopBytesLimiter creates a new limiter.
func NewNoopBytesLimiter() *NoopBytesLimiter {
	return &NoopBytesLimiter{}
}

// CheckBytes is a no-op byte checking function.
func (nbl *NoopBytesLimiter) CheckBytes() (bool, error) {
	return true, nil
}

// UseBytes is a no-op byte usage function.
func (nbl *NoopBytesLimiter) UseBytes(_ int64) error {
	return nil
}

// DB is an interface for a database communication.
type DB interface {
	// FetchBytes returns the amount of bytes used.
	FetchBytes(ctx context.Context) (int64, error)

	// IncreaseBytes increases the amount of bytes used.
	IncreaseBytes(ctx context.Context, usedBytes int64) error
}
