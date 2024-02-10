package request

import (
	"strings"
	"time"

	"github.com/rs/xid"
)

// Record is a record of a request.
type Record struct {
	// ID is the unique identifier of the request.
	ID xid.ID

	// Host is the host of the request.
	Host string

	// CreatedAt is the time when the request was created.
	CreatedAt time.Time
}

// NewRecord creates a new request record.
func NewRecord(host string) Record {
	return Record{
		ID:        xid.New(),
		Host:      strings.Split(host, ":")[0],
		CreatedAt: time.Now(),
	}
}
