package memory

import (
	"sync"

	"github.com/davseby/lwproxy/internal/request"
	"golang.org/x/exp/slog"
)

// Hub is a in-memory requests hub.
type Hub struct {
	log *slog.Logger

	mu   sync.RWMutex
	reqs []request.Record
}

// NewHub creates a new request hub.
func NewHub(log *slog.Logger) *Hub {
	return &Hub{
		log: log.With("thread", "requests-hub"),
	}
}

// Publish publishes a request record.
// NOTE: This could be extended with a limit of records to keep in the
// hub. If the limit is reached, the oldest records should be removed.
func (h *Hub) Publish(rec request.Record) error {
	h.log.With("host", rec.Host).
		Debug("publishing request record")

	h.mu.Lock()
	defer h.mu.Unlock()

	h.reqs = append(h.reqs, rec)

	return nil
}
