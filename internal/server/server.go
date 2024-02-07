package server

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/davseby/lwproxy/internal/request"
	"golang.org/x/exp/slog"
)

const (
	// _closeTimeout is the timeout for closing the server.
	_closeTimeout = 5 * time.Second

	// _targetDialTimeout is the timeout for dialing the target.
	_targetDialTimeout = 10 * time.Second

	// _connectionDeadline is the deadline for a connection.
	_connectionDeadline = 2 * time.Hour
)

// Server is a server.
type Server struct {
	log *slog.Logger

	srv *http.Server
	bl  BandwidthLimiter
	rb  RecordPublisher
}

// NewServer creates a new proxy server.
func NewServer(
	log *slog.Logger,
	bl BandwidthLimiter,
	rb RecordPublisher,
) (*Server, error) {
	s := &Server{
		log: log.With("thread", "server"),
		bl:  bl,
		rb:  rb,
	}

	s.srv = &http.Server{
		Addr:    ":8080",
		Handler: s,
	}

	return s, nil
}

// ListenAndServe listens for and serves connections. It blocks until the
// context is done or server listening procedure returns an error.
func (s *Server) ListenAndServe(ctx context.Context) {
	// NOTE: By having stop channel we can retry opening a server in case
	// the ListenAndServe method fails.
	stopCh := make(chan struct{})

	go func() {
		defer close(stopCh)

		err := s.srv.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.log.With("error", err).
				Error("listening and serving")
		}
	}()

	select {
	case <-stopCh:
	case <-ctx.Done():
		closureCtx, closureCancel := context.WithTimeout(context.Background(), _closeTimeout)
		defer closureCancel()

		err := s.srv.Shutdown(closureCtx)
		if err != nil {
			s.log.With("error", err).
				Error("shutting down server")
		}

		<-stopCh
	}
}

// ServeHTTP serves HTTP requests.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !s.bl.UseBytes(r.ContentLength) {
		http.Error(w, "bandwidth limit reached", http.StatusRequestEntityTooLarge)
		return
	}

	ctx, cancel := context.WithDeadline(r.Context(), time.Now().Add(_connectionDeadline))
	defer cancel()

	// NOTE: We publish a request record before handling the request.
	// This could be done in a separate goroutine to avoid blocking the
	// request handling as we don't know what will be done in the publish
	// method. However, in that case we should track the number
	// of spinned go routines and keep a limit on them. Other solution
	// could be to use a buffered channel communication. In case we publish
	// directly to a message broker, we should be able to avoid these
	// problems, except for the error handling.
	s.rb.Publish(request.NewRecord(r.Host, time.Now()))

	if r.Method == http.MethodConnect {
		s.handleTunneling(w, r.WithContext(ctx))
		return
	}

	s.handleRequest(w, r.WithContext(ctx))
}

// BandwidthLimiter is a bandwidth limiter.
type BandwidthLimiter interface {
	// UseBytes should use the given number of bytes from the limit.
	// It should return a boolean indicating if the byte limit hasn't
	// been reached.
	UseBytes(n int64) bool
}

// RecordPublisher is a record publisher.
type RecordPublisher interface {
	// Publish should publish a request record.
	Publish(rec request.Record) error
}
