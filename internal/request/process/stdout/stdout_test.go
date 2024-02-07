package stdout

import (
	"bytes"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/davseby/lwproxy/internal/request"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slog"
)

func Test_NewProcessor(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	proc := NewProcessor(log)
	require.NotNil(t, proc)
	assert.Equal(t, log.With("job", "requests-stdout-processor"), proc.log)
}

func Test_Processor_Handle(t *testing.T) {
	var buffer bytes.Buffer

	log := slog.New(slog.NewTextHandler(&buffer, nil))
	proc := &Processor{
		log: log,
	}

	rec := request.Record{
		ID:        xid.New(),
		Host:      "example.com",
		CreatedAt: time.Now(),
	}

	err := proc.Handle(rec)
	require.NoError(t, err)

	assert.Equal(
		t,
		fmt.Sprintf(
			"time=%s level=INFO msg=\"publishing request record\" id=%s host=%s\n",
			rec.CreatedAt.Format("2006-01-02T15:04:05.999Z07:00"),
			rec.ID.String(),
			rec.Host,
		),
		buffer.String(),
	)
}
