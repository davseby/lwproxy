// package intercept provides a way to intercept the read and write calls
// to a connection and checks the bytes used, potentially invalidating the
// connection.
//
//go:generate moq --stub -out 0moq_test.go . BytesLimiter:BytesLimiterMock conn:connMock listener:listenerMock
package intercept

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"strings"

	"golang.org/x/exp/slog"
)

// _bytesLimitExceeded is the message to send when the limit is exceeded.
const _bytesLimitExceeded = "bytes limit has been exceeded"

// Listener is an intercepted listener. It intercepts the accept call.
type Listener struct {
	listener

	log     *slog.Logger
	limiter BytesLimiter
}

// NewListener creates a new intercept listener.
func NewListener(
	log *slog.Logger,
	addr string,
	limiter BytesLimiter,
) (*Listener, error) {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}

	return &Listener{
		listener: l,
		log:      log.With("job", "intercept-listener"),
		limiter:  limiter,
	}, nil
}

// Accept waits for and returns the next connection to the listener. It
// creates an intercepted connection and checks the bytes limit.
func (l *Listener) Accept() (net.Conn, error) {
	conn, err := l.listener.Accept()
	if err != nil {
		return nil, err
	}

	ok, err := l.limiter.CheckBytes()

	switch {
	case err != nil:
		var internalErrorResponse = http.Response{
			StatusCode: http.StatusInternalServerError,
			ProtoMajor: 1,
			ProtoMinor: 1,
			Header: http.Header{
				"Content-Type": {"text/plain; charset=utf-8"},
			},
			Body: io.NopCloser(
				bytes.NewBufferString(
					strings.ToLower(
						http.StatusText(http.StatusInternalServerError),
					),
				),
			),
			ContentLength: int64(len(http.StatusText(http.StatusInternalServerError))),
		}

		if err := internalErrorResponse.Write(conn); err != nil {
			l.log.Error("failed to write internal error response", "error", err)
		}

		l.log.Error("failed to check bytes", "error", err)
	case !ok:
		var exceededLimitResponse = http.Response{
			StatusCode: http.StatusPaymentRequired,
			ProtoMajor: 1,
			ProtoMinor: 1,
			Header: http.Header{
				"Content-Type": {"text/plain; charset=utf-8"},
			},
			Body:          io.NopCloser(bytes.NewBufferString(_bytesLimitExceeded)),
			ContentLength: int64(len(_bytesLimitExceeded)),
		}

		if err := exceededLimitResponse.Write(conn); err != nil {
			l.log.Error("failed to write exceeded limit response", "error", err)
		}
	}

	if err != nil || !ok {
		err := conn.Close()
		if err != nil {
			l.log.Error("failed to close connection", "error", err)
		}

		// NOTE: We don't return an error here as that would cause the
		// listener to stop accepting connections and exit from the
		// serve method.
		return conn, nil
	}

	return &Conn{
		conn:    conn,
		limiter: l.limiter,
	}, nil
}

// Conn is an intercepted connection.
type Conn struct {
	conn

	limiter BytesLimiter
}

// Read reads data from the connection and uses the bytes limiter to
// increase the bytes used.
func (c *Conn) Read(b []byte) (int, error) {
	n, err := c.conn.Read(b)
	if err != nil {
		return 0, err
	}

	if err := c.limiter.UseBytes(int64(n)); err != nil {
		return 0, err
	}

	return n, nil
}

// Write writes data to the connection and uses the bytes limiter to
// increase the bytes used.
func (c *Conn) Write(b []byte) (int, error) {
	n, err := c.conn.Write(b)
	if err != nil {
		return 0, err
	}

	if err := c.limiter.UseBytes(int64(n)); err != nil {
		return 0, err
	}

	return n, nil
}

// BytesLimiter should be used to enforce bytes limitation to the proxy
// read and write operations.
type BytesLimiter interface {
	// CheckBytes should check if the bytes limit is exceeded.
	CheckBytes() (bool, error)

	// UsedBytes should use the provided number of bytes. If the limit is
	// exceeded, an error should be returned.
	UseBytes(n int64) error
}

// conn is an intercepted connection type. We redefine it here to mock it
// in the tests.
type conn net.Conn

// listener is an intercepted listener type. We redefine it here to mock it
// in the tests.
type listener net.Listener
