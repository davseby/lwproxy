package proxy

import (
	"context"
	"io"
	"net"
	"net/http"
	"sync"

	"golang.org/x/exp/slog"
)

// handleRequest handles a basic HTTP request.
func (p *Proxy) handleRequest(w http.ResponseWriter, r *http.Request) {
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

	deadline, ok := r.Context().Deadline()
	if ok {
		err := http.NewResponseController(w).SetWriteDeadline(deadline)
		if err != nil {
			p.log.Error("setting connection write deadline", slog.String("error", err.Error()))
		}
	}

	_, err = io.Copy(w, resp.Body)
	if err != nil && !silentError(err) {
		p.log.Error("handling request communication", slog.String("error", err.Error()))
	}
}

// handleTunneling handles tunneling.
func (p *Proxy) handleTunneling(w http.ResponseWriter, r *http.Request) {
	targetConn, err := net.DialTimeout("tcp", r.Host, _targetDialTimeout)
	if err != nil {
		http.Error(w, "target service is unreachable", http.StatusServiceUnavailable)
		return
	}

	// NOTE: We need to write the status header before hijacking the
	// connection.
	w.WriteHeader(http.StatusOK)

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijacking is not supported", http.StatusInternalServerError)
		return
	}

	baseConn, _, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, "cannot hijack a connection", http.StatusServiceUnavailable)
		return
	}

	p.establishCommunication(r.Context(), baseConn, targetConn)
}

// establishCommunication establishes communication between the base and
// target connections.
func (p *Proxy) establishCommunication(ctx context.Context, baseConn, targetConn net.Conn) {
	deadline, ok := ctx.Deadline()
	if ok {
		err := baseConn.SetDeadline(deadline)
		if err != nil {
			p.log.Error("setting base connection deadline", slog.String("error", err.Error()))
		}

		err = targetConn.SetDeadline(deadline)
		if err != nil {
			p.log.Error("setting target connection deadline", slog.String("error", err.Error()))
		}
	}

	closeConnections := func() {
		err := targetConn.Close()
		if err != nil && !silentError(err) {
			p.log.Error("closing target connection", slog.String("error", err.Error()))
		}

		err = baseConn.Close()
		if err != nil && !silentError(err) {
			p.log.Error("closing base connection", slog.String("error", err.Error()))
		}
	}

	var wg sync.WaitGroup

	wg.Add(1)

	go func() {
		defer wg.Done()

		_, err := io.Copy(baseConn, targetConn)
		if err != nil && !silentError(err) {
			p.log.Error("handling base to target communication", slog.String("error", err.Error()))
		}

		closeConnections()
	}()

	wg.Add(1)

	go func() {
		defer wg.Done()

		_, err := io.Copy(targetConn, baseConn)
		if err != nil && !silentError(err) {
			p.log.Error("handling target to base communication", slog.String("error", err.Error()))
		}

		closeConnections()
	}()

	wg.Wait()
}
