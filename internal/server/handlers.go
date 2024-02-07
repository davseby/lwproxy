package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sync"
)

// handleRequest handles a basic HTTP request.
func (s *Server) handleRequest(w http.ResponseWriter, r *http.Request) {
	resp, err := http.DefaultTransport.RoundTrip(r)
	if err != nil {
		http.Error(w, "target service is unreachable", http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()

	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	w.WriteHeader(resp.StatusCode)

	err = s.superviseTransfer(r.Context(), resp.Body, w)
	if err != nil && !silentError(err) {
		s.log.With("error", err).
			Error("handling request communication")
	}
}

// handleTunneling handles tunneling.
func (s *Server) handleTunneling(w http.ResponseWriter, r *http.Request) {
	targetConn, err := net.DialTimeout("tcp", r.Host, _targetDialTimeout)
	if err != nil {
		http.Error(w, "target service is unreachable", http.StatusServiceUnavailable)
		return
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijacking is not supported", http.StatusInternalServerError)
		return
	}

	// NOTE: We need to write the status header before hijacking the
	// connection.
	w.WriteHeader(http.StatusOK)

	baseConn, _, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, "cannot hijack a connection", http.StatusServiceUnavailable)
		return
	}

	s.establishCommunication(r.Context(), baseConn, targetConn)
}

// establishCommunication establishes communication between the base and
// target connections.
func (s *Server) establishCommunication(ctx context.Context, baseConn, targetConn net.Conn) {
	deadline, ok := ctx.Deadline()
	if ok {
		err := baseConn.SetDeadline(deadline)
		if err != nil {
			s.log.With("error", err).
				Error("setting base connection deadline")
		}

		err = targetConn.SetDeadline(deadline)
		if err != nil {
			s.log.With("error", err).
				Error("setting target connection deadline")
		}
	}

	closeConnections := func() {
		err := targetConn.Close()
		if err != nil && !silentError(err) {
			s.log.With("error", err).
				Error("closing target connection")
		}

		err = baseConn.Close()
		if err != nil && !silentError(err) {
			s.log.With("error", err).
				Error("closing base connection")
		}
	}

	var wg sync.WaitGroup

	wg.Add(1)

	go func() {
		defer wg.Done()

		err := s.superviseTransfer(ctx, baseConn, targetConn)
		if err != nil && !silentError(err) {
			s.log.With("error", err).
				Error("handling base to target communication")
		}

		closeConnections()
	}()

	wg.Add(1)

	go func() {
		defer wg.Done()

		err := s.superviseTransfer(ctx, targetConn, baseConn)
		if err != nil && !silentError(err) {
			s.log.With("error", err).
				Error("handling target to base communication")
		}

		closeConnections()
	}()

	wg.Wait()
}

// superviseTransfer reads from the source and writes to the destination until the
// context is canceled or the limit is reached.
func (s *Server) superviseTransfer(ctx context.Context, src io.Reader, dst io.Writer) error {
	buf := make([]byte, 1<<10) // 1 KB.

	for ctx.Err() == nil {
		n, err := src.Read(buf)
		switch {
		case err == nil:
			// OK.
		case errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed):
			return nil
		default:
			return fmt.Errorf("reading from source: %w", err)
		}

		if !s.bl.UseBytes(int64(n)) {
			break
		}

		_, err = dst.Write(buf[:n])
		if err != nil {
			return fmt.Errorf("writing to destination: %w", err)
		}
	}

	return nil
}

// silentError returns true if the error can be silenced.
func silentError(err error) bool {
	return errors.Is(err, context.Canceled) ||
		errors.Is(err, os.ErrDeadlineExceeded) ||
		errors.Is(err, net.ErrClosed)
}
