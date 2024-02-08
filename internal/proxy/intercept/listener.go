package intercept

import (
	"net"
)

// Listener is an intercepted listener. It intercepts the accept call.
type Listener struct {
	net.Listener

	bl BytesLimiter
	cp *ControlPoint
}

// NewListener intercepts the listen call.
func NewListener(
	network, addr string,
	bl BytesLimiter,
	cp *ControlPoint,
) (net.Listener, error) {
	l, err := net.Listen(network, addr)
	if err != nil {
		return l, err
	}

	return &Listener{
		Listener: l,
		bl:       bl,
		cp:       cp,
	}, nil
}

// Accept waits for and returns the next connection to the listener. It
// intercepts the accept call.
func (l *Listener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return conn, err
	}

	ok, err := l.bl.CheckBytes()
	if err != nil {
		return nil, err
	}

	if !ok {
		l.cp.Add(conn.RemoteAddr().String())
	}

	return &Conn{
		Conn: conn,
		bl:   l.bl,
		cp:   l.cp,
	}, nil
}

// Conn is an intercepted connection. It tracks the bytes written
// and read from the connection.
type Conn struct {
	net.Conn

	bl BytesLimiter
	cp *ControlPoint
}

// Read reads data from the connection. It intercepts the read call.
// TODO: Check context usage.
func (c *Conn) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	if err != nil {
		return 0, err
	}

	if !c.cp.Has(c.RemoteAddr().String()) {
		if err := c.bl.UseBytes(int64(n)); err != nil {
			return 0, err
		}
	}

	return n, nil
}

// Write writes data to the connection. It intercepts the write call.
// TODO: Check context usage.
func (c *Conn) Write(b []byte) (int, error) {
	n, err := c.Conn.Write(b)
	if err != nil {
		return 0, err
	}

	if !c.cp.Has(c.RemoteAddr().String()) {
		if err := c.bl.UseBytes(int64(n)); err != nil {
			return 0, err
		}
	}

	return n, nil
}

// BytesLimiter should be used to enforce bytes limitation to the the proxy
// read and write operations.
type BytesLimiter interface {
	// CheckBytes should check if the bytes limit is exceeded.
	CheckBytes() (bool, error)

	// UsedBytes should use the provided number of bytes.
	UseBytes(n int64) error
}
