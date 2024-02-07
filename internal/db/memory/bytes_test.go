package memory

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_DB_FetchBytes(t *testing.T) {
	db := DB{
		bytes: &atomic.Int64{},
	}

	db.bytes.Add(5)

	bytes, err := db.FetchBytes(context.Background())
	require.NoError(t, err)
	assert.Equal(t, bytes, int64(5))
}

func Test_DB_IncreaseBytes(t *testing.T) {
	db := DB{
		bytes: &atomic.Int64{},
	}

	err := db.IncreaseBytes(context.Background(), 5)
	require.NoError(t, err)

	assert.Equal(t, db.bytes.Load(), int64(5))
}
