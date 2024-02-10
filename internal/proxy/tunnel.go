package proxy

import (
	"context"
	"io"
	"net"
	"net/http"
	"sync"
)

// tunnelingHandler handles tunneling (e.g proxying).
func (p *Proxy) tunnelingHandler(w http.ResponseWriter, r *http.Request, secure bool) {
	targetConn, err := net.DialTimeout("tcp", r.Host, _targetDialTimeout)
	if err != nil {
		http.Error(w, "target service is unreachable", http.StatusServiceUnavailable)
		return
	}

	if secure {
		// NOTE: We need to write the status header before hijacking the
		// connection. This will tell the client that we've established the
		// connection between the client and the target server. Used
		// for TLS.
		w.WriteHeader(http.StatusOK)
	} else {
		err = r.Write(targetConn)
		if err != nil {
			http.Error(w, "writing request to the target service", http.StatusInternalServerError)
			return
		}
	}

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
// target connections. This also handles the deadline for the communication
// and closes the connections when the communication is done.
func (p *Proxy) establishCommunication(ctx context.Context, baseConn, targetConn net.Conn) {
	deadline, ok := ctx.Deadline()
	if ok {
		err := baseConn.SetDeadline(deadline)
		if err != nil {
			p.silentError(err, "setting base connection deadline")
		}

		err = targetConn.SetDeadline(deadline)
		if err != nil {
			p.silentError(err, "setting target connection deadline")
		}
	}

	closeConnections := func() {
		err := targetConn.Close()
		if err != nil {
			p.silentError(err, "closing target connection")
		}

		err = baseConn.Close()
		if err != nil {
			p.silentError(err, "closing base connection")
		}
	}

	var wg sync.WaitGroup

	wg.Add(1)

	go func() {
		defer wg.Done()

		_, err := io.Copy(baseConn, targetConn)
		if err != nil {
			p.silentError(err, "handling base to target communication")
		}

		closeConnections()
	}()

	wg.Add(1)

	go func() {
		defer wg.Done()

		_, err := io.Copy(targetConn, baseConn)
		if err != nil {
			p.silentError(err, "handling target to base communication")
		}

		closeConnections()
	}()

	wg.Wait()
}
