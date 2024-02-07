// package intercept provides a way to intercept the read and write calls
// to a connection and checks the bytes used, potentially invalidating the
// connection using a control point.
//
//go:generate moq --stub -out 0moq_test.go . BytesLimiter:BytesLimiterMock conn:connMock listener:listenerMock
package intercept

import (
	"net"
)

// Listener is an intercepted listener. It intercepts the accept call.
type Listener struct {
	listener

	limiter BytesLimiter
	control *Control
}

// NewListener intercepts the listen call.
func NewListener(
	addr string,
	limiter BytesLimiter,
	control *Control,
) (*Listener, error) {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}

	return &Listener{
		listener: l,
		limiter:  limiter,
		control:  control,
	}, nil
}

// Accept waits for and returns the next connection to the listener. It
// intercepts the accept call.
func (l *Listener) Accept() (net.Conn, error) {
	conn, err := l.listener.Accept()
	if err != nil {
		return nil, err
	}

	ok, err := l.limiter.CheckBytes()
	if err != nil {
		return nil, err
	}

	if !ok {
		l.control.Add(conn.RemoteAddr().String())
	}

	return &Conn{
		conn:      conn,
		limiter:   l.limiter,
		skipCheck: !ok,
	}, nil
}

// Conn is an intercepted connection. It tracks the bytes written
// and read from the connection.
type Conn struct {
	conn

	skipCheck bool
	limiter   BytesLimiter
}

// Read reads data from the connection. It intercepts the read call.
func (c *Conn) Read(b []byte) (int, error) {
	n, err := c.conn.Read(b)
	if err != nil {
		return 0, err
	}

	// NOTE: We could probably move this to a separate routine where a
	// buffered channel is used to send the number of bytes read. This
	// would allow us to avoid the lock  increasing the write and read
	// performance, however we risk using too much data and exceeding the
	// limit. Depending on the project requirements, this could be
	// adjusted to meet them
	if !c.skipCheck {
		if err := c.limiter.UseBytes(int64(n)); err != nil {
			return 0, err
		}
	}

	return n, nil
}

// Write writes data to the connection. It intercepts the write call.
func (c *Conn) Write(b []byte) (int, error) {
	n, err := c.conn.Write(b)
	if err != nil {
		return 0, err
	}

	// NOTE: Comment applies same as for the read.
	if !c.skipCheck {
		if err := c.limiter.UseBytes(int64(n)); err != nil {
			return 0, err
		}
	}

	return n, nil
}

// BytesLimiter should be used to enforce bytes limitation to the proxy
// read and write operations.
type BytesLimiter interface {
	// CheckBytes should check if the bytes limit is exceeded.
	CheckBytes() (bool, error)

	// UsedBytes should use the provided number of bytes.
	UseBytes(n int64) error
}

// conn is an intercepted connection type. We redefine it here to mock it
// in the tests.
type conn net.Conn

// listener is an intercepted listener type. We redefine it here to mock it
// in the tests.
type listener net.Listener
