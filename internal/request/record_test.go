package request

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_NewRecord(t *testing.T) {
	rec := NewRecord("example.com")

	assert.NotEmpty(t, rec.ID)
	assert.Equal(t, "example.com", rec.Host)
	assert.WithinDuration(t, time.Now(), rec.CreatedAt, time.Second*5)
}
