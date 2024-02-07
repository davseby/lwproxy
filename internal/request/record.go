package request

import (
	"strings"
	"time"
)

// Record is a record of a request.
type Record struct {
	// Host is the host of the request.
	Host string

	// CreatedAt is the time when the request was created.
	CreatedAt time.Time
}

// NewRecord creates a new request record.
func NewRecord(host string, createdAt time.Time) Record {
	return Record{
		Host:      strings.Split(host, ":")[0],
		CreatedAt: createdAt,
	}
}
