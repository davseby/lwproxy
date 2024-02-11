package intercept

import (
	"bytes"
	"io"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slog"
)

func Test_NewListener(t *testing.T) {
	blm := &BytesLimiterMock{}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	// error
	l, err := NewListener(log, "9999", blm)
	require.Error(t, err)
	assert.Nil(t, l)

	// success
	l, err = NewListener(log, ":9999", blm)
	require.Empty(t, err)
	require.NotNil(t, l)
	assert.Same(t, blm, l.limiter)
	assert.Equal(t, log.With("job", "intercept-listener"), l.log)
}

func Test_Listener_Accept(t *testing.T) {
	stubListener := func(conn net.Conn, err error) *listenerMock {
		return &listenerMock{
			AcceptFunc: func() (net.Conn, error) {
				return conn, err
			},
		}
	}

	stubConn := func(werr, cerr error) *connMock {
		return &connMock{
			WriteFunc: func(n []byte) (int, error) {
				return len(n), werr
			},
			CloseFunc: func() error {
				return cerr
			},
		}
	}

	stubBytesLimiter := func(ok bool, err error) *BytesLimiterMock {
		return &BytesLimiterMock{
			CheckBytesFunc: func() (bool, error) {
				return ok, err
			},
		}
	}

	type check func(*testing.T, *listenerMock, *connMock, *BytesLimiterMock)

	wasListenerAcceptCalled := func(called bool) check {
		return func(t *testing.T, l *listenerMock, _ *connMock, _ *BytesLimiterMock) {
			if called {
				assert.Len(t, l.AcceptCalls(), 1)
				return
			}

			assert.Len(t, l.AcceptCalls(), 0)
		}
	}

	wasCheckBytesCalled := func(called bool) check {
		return func(t *testing.T, _ *listenerMock, _ *connMock, lim *BytesLimiterMock) {
			if called {
				assert.Len(t, lim.CheckBytesCalls(), 1)
				return
			}

			assert.Len(t, lim.CheckBytesCalls(), 0)
		}
	}

	wasConnWriteCalled := func(calls int) check {
		return func(t *testing.T, _ *listenerMock, c *connMock, _ *BytesLimiterMock) {
			assert.Len(t, c.WriteCalls(), calls)
		}
	}

	wasConnCloseCalled := func(called bool) check {
		return func(t *testing.T, _ *listenerMock, c *connMock, _ *BytesLimiterMock) {
			if called {
				assert.Len(t, c.CloseCalls(), 1)
				return
			}

			assert.Len(t, c.CloseCalls(), 0)
		}
	}

	type tcase struct {
		Listener   *listenerMock
		Limiter    *BytesLimiterMock
		Success    bool
		Error      error
		Conn       *connMock
		LogOutputs []string
		Checks     []check
	}

	tests := map[string]tcase{
		"listener.Accept returns an error": func() tcase {
			cm := stubConn(nil, nil)
			lim := stubBytesLimiter(false, nil)

			return tcase{
				Listener:   stubListener(nil, assert.AnError),
				Limiter:    lim,
				Success:    false,
				Error:      assert.AnError,
				Conn:       cm,
				LogOutputs: nil,
				Checks: []check{
					wasListenerAcceptCalled(true),
					wasCheckBytesCalled(false),
					wasConnWriteCalled(0),
					wasConnCloseCalled(false),
				},
			}
		}(),
		"limiter.CheckBytes returns an error": func() tcase {
			cm := stubConn(nil, nil)
			lim := stubBytesLimiter(false, assert.AnError)

			return tcase{
				Listener: stubListener(cm, nil),
				Limiter:  lim,
				Success:  false,
				Conn:     cm,
				LogOutputs: []string{
					"level=ERROR msg=\"failed to check bytes\" error=\"assert.AnError general error for testing\"\n",
				},
				Checks: []check{
					wasListenerAcceptCalled(true),
					wasCheckBytesCalled(true),
					wasConnWriteCalled(9),
					wasConnCloseCalled(true),
				},
			}
		}(),
		"limiter.CheckBytes and conn.Write returns an error": func() tcase {
			cm := stubConn(assert.AnError, nil)
			lim := stubBytesLimiter(false, assert.AnError)

			return tcase{
				Listener: stubListener(cm, nil),
				Limiter:  lim,
				Success:  false,
				Conn:     cm,
				LogOutputs: []string{
					"level=ERROR msg=\"failed to check bytes\" error=\"assert.AnError general error for testing\"\n",
					"level=ERROR msg=\"failed to write internal error response\" error=\"assert.AnError general error for testing\"\n",
				},
				Checks: []check{
					wasListenerAcceptCalled(true),
					wasCheckBytesCalled(true),
					wasConnWriteCalled(1),
					wasConnCloseCalled(true),
				},
			}
		}(),
		"conn.Write returns an error": func() tcase {
			cm := stubConn(assert.AnError, nil)
			lim := stubBytesLimiter(false, nil)

			return tcase{
				Listener: stubListener(cm, nil),
				Limiter:  lim,
				Success:  false,
				Conn:     cm,
				LogOutputs: []string{
					"level=ERROR msg=\"failed to write exceeded limit response\" error=\"assert.AnError general error for testing\"\n",
				},
				Checks: []check{
					wasListenerAcceptCalled(true),
					wasCheckBytesCalled(true),
					wasConnWriteCalled(1),
					wasConnCloseCalled(true),
				},
			}
		}(),
		"conn.Write and conn.Close returns an error": func() tcase {
			cm := stubConn(assert.AnError, assert.AnError)
			lim := stubBytesLimiter(false, nil)

			return tcase{
				Listener: stubListener(cm, nil),
				Limiter:  lim,
				Success:  false,
				Conn:     cm,
				LogOutputs: []string{
					"level=ERROR msg=\"failed to write exceeded limit response\" error=\"assert.AnError general error for testing\"\n",
					"level=ERROR msg=\"failed to close connection\" error=\"assert.AnError general error for testing\"\n",
				},
				Checks: []check{
					wasListenerAcceptCalled(true),
					wasCheckBytesCalled(true),
					wasConnWriteCalled(1),
					wasConnCloseCalled(true),
				},
			}
		}(),
		"Successfully rejected a connection due to the limits breach": func() tcase {
			cm := stubConn(nil, nil)
			lim := stubBytesLimiter(false, nil)

			return tcase{
				Listener:   stubListener(cm, nil),
				Limiter:    lim,
				Success:    false,
				Conn:       cm,
				LogOutputs: nil,
				Checks: []check{
					wasListenerAcceptCalled(true),
					wasCheckBytesCalled(true),
					wasConnWriteCalled(9), // response write makes 9 calls
					wasConnCloseCalled(true),
				},
			}
		}(),
		"Successfully accepted a connection": func() tcase {
			cm := stubConn(nil, nil)
			lim := stubBytesLimiter(true, nil)

			return tcase{
				Listener:   stubListener(cm, nil),
				Limiter:    lim,
				Success:    true,
				Conn:       cm,
				LogOutputs: nil,
				Checks: []check{
					wasListenerAcceptCalled(true),
					wasCheckBytesCalled(true),
					wasConnWriteCalled(0),
					wasConnCloseCalled(false),
				},
			}
		}(),
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var buffer bytes.Buffer

			l := &Listener{
				log:      slog.New(slog.NewTextHandler(&buffer, nil)),
				listener: test.Listener,
				limiter:  test.Limiter,
			}

			conn, err := l.Accept()

			for _, check := range test.Checks {
				check(t, test.Listener, test.Conn, test.Limiter)
			}

			if test.Error != nil {
				assert.Equal(t, test.Error, err)
				assert.Nil(t, conn)

				return
			}

			require.NoError(t, err)

			for i := range test.LogOutputs {
				assert.Contains(t, buffer.String(), test.LogOutputs[i])
			}

			if test.LogOutputs == nil {
				assert.Empty(t, buffer.String())
			}

			if test.Success {
				assert.Equal(t, &Conn{
					conn:    test.Conn,
					limiter: test.Limiter,
				}, conn)
			} else {
				assert.Equal(t, test.Conn, conn)
			}
		})
	}
}

func Test_Conn_Read(t *testing.T) {
	stubConn := func(length int, err error) *connMock {
		return &connMock{
			ReadFunc: func(_ []byte) (int, error) {
				return length, err
			},
		}
	}

	stubBytesLimiter := func(err error) *BytesLimiterMock {
		return &BytesLimiterMock{
			UseBytesFunc: func(_ int64) error {
				return err
			},
		}
	}

	type check func(*testing.T, *connMock, *BytesLimiterMock)

	wasConnReadCalled := func(b []byte) check {
		return func(t *testing.T, c *connMock, _ *BytesLimiterMock) {
			require.Len(t, c.ReadCalls(), 1)
			assert.Equal(t, b, c.ReadCalls()[0].B)
		}
	}

	wasBytesLimiterUseBytesCalled := func(called bool, size int64) check {
		return func(t *testing.T, _ *connMock, lim *BytesLimiterMock) {
			if called {
				assert.Len(t, lim.UseBytesCalls(), 1)
				assert.Equal(t, size, lim.UseBytesCalls()[0].N)

				return
			}

			assert.Len(t, lim.UseBytesCalls(), 0)
		}
	}

	tests := map[string]struct {
		Conn      *connMock
		SkipCheck bool
		Limiter   *BytesLimiterMock
		Size      int
		Error     error
		Checks    []check
	}{
		"conn.Read returns an error": {
			Conn:    stubConn(0, assert.AnError),
			Limiter: stubBytesLimiter(nil),
			Error:   assert.AnError,
			Checks: []check{
				wasConnReadCalled([]byte{1, 2, 3}),
				wasBytesLimiterUseBytesCalled(false, 0),
			},
		},
		"limiter.UseBytes returns an error": {
			Conn:    stubConn(0, nil),
			Limiter: stubBytesLimiter(assert.AnError),
			Error:   assert.AnError,
			Checks: []check{
				wasConnReadCalled([]byte{1, 2, 3}),
				wasBytesLimiterUseBytesCalled(true, 0),
			},
		},
		"Successfully read from a connection": {
			Conn:    stubConn(3, nil),
			Limiter: stubBytesLimiter(nil),
			Size:    3,
			Checks: []check{
				wasConnReadCalled([]byte{1, 2, 3}),
				wasBytesLimiterUseBytesCalled(true, 3),
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c := &Conn{
				conn:    test.Conn,
				limiter: test.Limiter,
			}

			n, err := c.Read([]byte{1, 2, 3})

			for _, check := range test.Checks {
				check(t, test.Conn, test.Limiter)
			}

			assert.Equal(t, test.Error, err)
			assert.Equal(t, test.Size, n)
		})
	}
}

func Test_Conn_Write(t *testing.T) {
	stubConn := func(length int, err error) *connMock {
		return &connMock{
			WriteFunc: func(_ []byte) (int, error) {
				return length, err
			},
		}
	}

	stubBytesLimiter := func(err error) *BytesLimiterMock {
		return &BytesLimiterMock{
			UseBytesFunc: func(_ int64) error {
				return err
			},
		}
	}

	type check func(*testing.T, *connMock, *BytesLimiterMock)

	wasConnWriteCalled := func(b []byte) check {
		return func(t *testing.T, c *connMock, _ *BytesLimiterMock) {
			require.Len(t, c.WriteCalls(), 1)
			assert.Equal(t, b, c.WriteCalls()[0].B)
		}
	}

	wasBytesLimiterUseBytesCalled := func(called bool, size int64) check {
		return func(t *testing.T, _ *connMock, bl *BytesLimiterMock) {
			if called {
				assert.Len(t, bl.UseBytesCalls(), 1)
				assert.Equal(t, size, bl.UseBytesCalls()[0].N)

				return
			}

			assert.Len(t, bl.UseBytesCalls(), 0)
		}
	}

	tests := map[string]struct {
		Conn    *connMock
		Limiter *BytesLimiterMock
		Size    int
		Error   error
		Checks  []check
	}{
		"conn.Write returns an error": {
			Conn:    stubConn(0, assert.AnError),
			Limiter: stubBytesLimiter(nil),
			Error:   assert.AnError,
			Checks: []check{
				wasConnWriteCalled([]byte{1, 2, 3}),
				wasBytesLimiterUseBytesCalled(false, 0),
			},
		},
		"limiter.UseBytes returns an error": {
			Conn:    stubConn(0, nil),
			Limiter: stubBytesLimiter(assert.AnError),
			Error:   assert.AnError,
			Checks: []check{
				wasConnWriteCalled([]byte{1, 2, 3}),
				wasBytesLimiterUseBytesCalled(true, 0),
			},
		},
		"Successfully read from a connection": {
			Conn:    stubConn(3, nil),
			Limiter: stubBytesLimiter(nil),
			Size:    3,
			Checks: []check{
				wasConnWriteCalled([]byte{1, 2, 3}),
				wasBytesLimiterUseBytesCalled(true, 3),
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c := &Conn{
				conn:    test.Conn,
				limiter: test.Limiter,
			}

			n, err := c.Write([]byte{1, 2, 3})

			for _, check := range test.Checks {
				check(t, test.Conn, test.Limiter)
			}

			assert.Equal(t, test.Error, err)
			assert.Equal(t, test.Size, n)
		})
	}
}
