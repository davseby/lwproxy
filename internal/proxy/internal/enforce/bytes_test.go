package enforce

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewBytesLimiter(t *testing.T) {
	dbMock := &DBMock{}

	bl := NewBytesLimiter(dbMock, 500)
	require.NotNil(t, bl)
	assert.Equal(t, int64(500), bl.maxBytes)
	assert.Equal(t, dbMock, bl.db)
}

func Test_BytesLimiter_CheckBytes(t *testing.T) {
	stubDatabase := func(bytes int64, err error) *DBMock {
		return &DBMock{
			FetchBytesFunc: func(_ context.Context) (int64, error) {
				return bytes, err
			},
		}
	}

	tests := map[string]struct {
		DB       *DBMock
		MaxBytes int64
		Result   bool
		Error    error
	}{
		"db.FetchBytes returned an error": {
			DB:       stubDatabase(0, assert.AnError),
			MaxBytes: 500,
			Error:    assert.AnError,
		},
		"Successfully executed, however check did not pass": {
			DB:       stubDatabase(600, nil),
			MaxBytes: 500,
		},
		"Successfully executed and check passes": {
			DB:       stubDatabase(300, nil),
			MaxBytes: 500,
			Result:   true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			bl := &BytesLimiter{
				db:       test.DB,
				maxBytes: test.MaxBytes,
			}

			ok, err := bl.CheckBytes()
			assert.Equal(t, test.Result, ok)
			assert.Equal(t, test.Error, err)

			assert.Len(t, test.DB.FetchBytesCalls(), 1)
		})
	}
}

func Test_BytesLimiter_UseBytes(t *testing.T) {
	stubDB := func(bytes int64, fbErr, ibErr error) *DBMock {
		return &DBMock{
			FetchBytesFunc: func(_ context.Context) (int64, error) {
				return bytes, fbErr
			},
			IncreaseBytesFunc: func(_ context.Context, _ int64) error {
				return ibErr
			},
		}
	}

	type check func(t *testing.T, db *DBMock)

	wasDBFetchBytesCalled := func(called bool) check {
		return func(t *testing.T, db *DBMock) {
			var count int

			if called {
				count = 1
			}

			assert.Len(t, db.FetchBytesCalls(), count)
		}
	}

	wasDBIncreaseBytesCalled := func(called bool, bytes int64) check {
		return func(t *testing.T, db *DBMock) {
			var count int

			if called {
				count = 1
			}

			require.Len(t, db.IncreaseBytesCalls(), count)

			if called {
				assert.Equal(t, bytes, db.IncreaseBytesCalls()[0].UsedBytes)
			}
		}
	}

	tests := map[string]struct {
		DB        *DBMock
		MaxBytes  int64
		UsedBytes int64
		Error     error
		Checks    []check
	}{
		"db.FetchBytes returned an error": {
			DB:       stubDB(0, assert.AnError, nil),
			MaxBytes: 500,
			Error:    assert.AnError,
			Checks: []check{
				wasDBFetchBytesCalled(true),
				wasDBIncreaseBytesCalled(false, 0),
			},
		},
		"db.IncreaseBytes returned an error": {
			DB:        stubDB(0, nil, assert.AnError),
			MaxBytes:  500,
			UsedBytes: 200,
			Error:     assert.AnError,
			Checks: []check{
				wasDBFetchBytesCalled(true),
				wasDBIncreaseBytesCalled(true, 200),
			},
		},
		"Successfully executed, however overflow was reached": {
			DB:        stubDB(400, nil, nil),
			MaxBytes:  500,
			UsedBytes: 300,
			Error:     ErrLimitExceeded,
			Checks: []check{
				wasDBFetchBytesCalled(true),
				wasDBIncreaseBytesCalled(true, 300),
			},
		},
		"Successfully executed, no overflow was reached": {
			DB:        stubDB(100, nil, nil),
			MaxBytes:  500,
			UsedBytes: 300,
			Checks: []check{
				wasDBFetchBytesCalled(true),
				wasDBIncreaseBytesCalled(true, 300),
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			bl := &BytesLimiter{
				db:       test.DB,
				maxBytes: test.MaxBytes,
			}

			assert.Equal(t, test.Error, bl.UseBytes(test.UsedBytes))

			for _, check := range test.Checks {
				check(t, test.DB)
			}
		})
	}
}

func Test_NewNoopBytesLimiter(t *testing.T) {
	bl := NewNoopBytesLimiter()
	require.NotNil(t, bl)
}

func Test_NoopBytesLimiter_CheckBytes(t *testing.T) {
	bl := NoopBytesLimiter{}

	ok, err := bl.CheckBytes()
	require.NoError(t, err)
	assert.True(t, ok)
}

func Test_NoopBytesLimiter_UseBytes(t *testing.T) {
	bl := NoopBytesLimiter{}

	require.NoError(t, bl.UseBytes(500))
}
