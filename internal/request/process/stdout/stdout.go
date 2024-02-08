package stdout

import (
	"github.com/davseby/lwproxy/internal/request"
	"golang.org/x/exp/slog"
)

// Processor is a requests processor that uses standard output to log
// requests.
type Processor struct {
	log *slog.Logger
}

// NewProcessor creates a new request processor.
func NewProcessor(log *slog.Logger) *Processor {
	return &Processor{
		log: log.With("job", "requests-stdout-processor"),
	}
}

// Process processes a request record.
func (p *Processor) Create(rec request.Record) error {
	p.log.With("id", rec.ID.String()).
		With("host", rec.Host).
		Info("publishing request record")

	return nil
}
