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

// Handle handles a new record.
func (p *Processor) Handle(rec request.Record) error {
	p.log.Info(
		"publishing request record",
		slog.String("id", rec.ID.String()),
		slog.String("host", rec.Host),
	)

	return nil
}
