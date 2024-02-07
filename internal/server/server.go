package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"golang.org/x/exp/slog"
)

// _closeTimeout is the timeout for closing the server.
const _closeTimeout = 5 * time.Second

// Server is a server.
type Server struct {
	logger *slog.Logger

	server *http.Server
	client *http.Client
}

// NewServer creates a new server.
func NewServer(logger *slog.Logger) (*Server, error) {
	s := &Server{
		logger: logger.With("thread", "server"),
		client: http.DefaultClient,
	}

	s.server = &http.Server{
		Addr:    ":8080",
		Handler: s,
	}

	return s, nil
}

// ListenAndServe listens for and serves connections.
func (s *Server) ListenAndServe(ctx context.Context) {
	stopCh := make(chan struct{})

	// NOTE: In case we need to exit due to application shutdown, we need
	// to close the server and return the error.
	go func() {
		defer close(stopCh)

		err := s.server.ListenAndServe()
		if err != nil && errors.Is(err, http.ErrServerClosed) {
			s.logger.With("error", err).
				Error("listening and serving")
		}
	}()

	select {
	case <-stopCh:
		return
	case <-ctx.Done():
		closureCtx, closureCancel := context.WithTimeout(context.Background(), _closeTimeout)
		defer closureCancel()

		err := s.server.Shutdown(closureCtx)
		if err != nil {
			s.logger.With("error", err).
				Error("shutting down server")
		}

		<-stopCh
	}
}

// ServeHTTP serves HTTP.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.logger.Info("serving http")

	if r.Method == http.MethodConnect {
		s.handleTunneling(w, r)
		return
	}

	s.handleHTTPRequest(w, r)
	//switch r.URL.Scheme {
	//case "http":
	//case "https":
	//	http.Error(w, "https is not supported", http.StatusInternalServerError)
	//}
}

func (s *Server) handleTunneling(w http.ResponseWriter, r *http.Request) {
	dest_conn, err := net.DialTimeout("tcp", r.Host, 10*time.Second)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	fmt.Println("handled tunneling")

	w.WriteHeader(http.StatusOK)

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		return
	}

	client_conn, _, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
	}

	go negiotateCommunication(dest_conn, client_conn)
	go negiotateCommunication(client_conn, dest_conn)

}

func (s *Server) handleHTTPRequest(w http.ResponseWriter, r *http.Request) {
	// NOTE: RequestURI is empty to avoid request loop.
	r.RequestURI = ""

	s.logger.With("scheme", r.URL.Scheme).
		With("url", r.URL.String()).Info("received request")

	r.URL.Scheme = "https"

	fmt.Println("#v", r)

	resp, err := s.client.Do(r)
	if err != nil {
		s.logger.With("error", err).
			Error("sending request")

		http.Error(w, "internal server error", http.StatusInternalServerError)

		return
	}
	defer resp.Body.Close()

	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	w.WriteHeader(resp.StatusCode)

	io.Copy(w, resp.Body)
}

// handleConnection handles a connection.
//
//	func (s *Server) handleConnection(ctx context.Context, baseConn net.Conn) {
//		targetConn, req, err := dialTarget(baseConn)
//		if err != nil {
//			s.logger.With("error", err).
//				Error("setting up target")
//
//			err := baseConn.Close()
//			if err != nil && !errors.Is(err, net.ErrClosed) {
//				s.logger.With("error", err).
//					Error("closing base connection")
//			}
//
//			return
//		}
//
//		s.logger.With("request", req.URL.Host).Info("handling connection")
//
//		closeConnections := func() {
//			err := targetConn.Close()
//			if err != nil && !errors.Is(err, net.ErrClosed) {
//				s.logger.With("error", err).
//					Error("closing target connection")
//			}
//
//			err = baseConn.Close()
//			if err != nil && !errors.Is(err, net.ErrClosed) {
//				s.logger.With("error", err).
//					Error("closing base connection")
//			}
//		}
//
//		var wg sync.WaitGroup
//
//		wg.Add(1)
//
//		go func() {
//			defer wg.Done()
//
//			err := negiotateCommunication(ctx, baseConn, targetConn)
//			if err != nil {
//				s.logger.With("error", err).
//					Error("negiotating communication")
//			}
//
//			closeConnections()
//		}()
//
//		wg.Add(1)
//
//		go func() {
//			defer wg.Done()
//
//			err := negiotateCommunication(ctx, targetConn, baseConn)
//			if err != nil {
//				s.logger.With("error", err).
//					Error("negiotating communication")
//			}
//
//			closeConnections()
//		}()
//
//		wg.Wait()
//
//		s.logger.With("request", req.URL.Host).Info("connection handled")
//	}
//
// // dialTarget sets up the target connection.
//
//	func dialTarget(baseConn net.Conn) (net.Conn, *http.Request, error) {
//		buf := make([]byte, 65535)
//
//		n, err := baseConn.Read(buf)
//		if err != nil {
//			return nil, nil, fmt.Errorf("reading initial request: %w", err)
//		}
//
//		req, err := http.ReadRequest(
//			bufio.NewReader(
//				bytes.NewBuffer(buf[:n]),
//			),
//		)
//		if err != nil {
//			return nil, nil, fmt.Errorf("deriving target: %w", err)
//		}
//
//		targetConn, err := net.DialTimeout("tcp", req.URL.Host, time.Second*5)
//		if err != nil {
//			return nil, nil, fmt.Errorf("dialing target: %w", err)
//		}
//
//		rbuf := make([]byte, 65535)
//
//		err = req.Write(bytes.NewBuffer(rbuf))
//		if err != nil {
//			return nil, nil, fmt.Errorf("writing initial request: %w", err)
//		}
//
//		_, err = targetConn.Write(rbuf[:n])
//		if err != nil {
//			return nil, nil, fmt.Errorf("writing initial request: %w", err)
//		}
//
//		return targetConn, req, nil
//	}
//
// negiotateCommunication negiotates communication between source and
// destination connections.
func negiotateCommunication(src, dst net.Conn) error {
	io.Copy(dst, src)

	return nil
}
