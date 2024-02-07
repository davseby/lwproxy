// package proxy provides a proxy server implementation for the proxy service.
package proxy

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/davseby/lwproxy/internal/proxy/internal/enforce"
	"github.com/davseby/lwproxy/internal/proxy/internal/intercept"
	"github.com/davseby/lwproxy/internal/request"
	"golang.org/x/exp/slog"
)

const (
	// _closeTimeout is the timeout for closing the proxy.
	_closeTimeout = 5 * time.Second

	// _targetDialTimeout is the timeout for dialing the target.
	_targetDialTimeout = 10 * time.Second

	// _connectionTimeout is the timeout for a connection.
	_connectionTimeout = 2 * time.Hour

	// _readHeaderTimeout is the timeout for reading the header.
	_readHeaderTimeout = 5 * time.Second
)

// Proxy is a proxy server.
type Proxy struct {
	log *slog.Logger

	srv *http.Server

	rec     Recorder
	limiter intercept.BytesLimiter

	cfg     Config
	control *intercept.Control
}

// Config holds the settings for the proxy server.
type Config struct {
	// Addr is the address to listen on.
	Addr string `default:":8081"`

	// MaxBytes is the maximum amount of bytes that can be used.
	MaxBytes int64 `default:"2000000000"`

	Auth struct {
		// Username is the username for basic authentication.
		Username string `default:"admin"`

		// Password is the password for basic authentication.
		Password string `default:"admin"`
	}
}

// NewProxy creates a new proxy server.
func NewProxy(
	log *slog.Logger,
	rec Recorder,
	db DB,
	cfg Config,
) (*Proxy, error) {
	var bl intercept.BytesLimiter = enforce.NewNoopBytesLimiter()

	if cfg.MaxBytes > 0 {
		bl = enforce.NewBytesLimiter(db, cfg.MaxBytes)
	}

	p := &Proxy{
		log:     log.With("job", "proxy"),
		rec:     rec,
		cfg:     cfg,
		limiter: bl,
		control: intercept.NewControl(),
	}

	p.srv = &http.Server{
		Addr:              cfg.Addr,
		Handler:           http.HandlerFunc(p.authHandler),
		ReadHeaderTimeout: _readHeaderTimeout,

		// NOTE: We need to set TLSNextProto to an empty map to disable
		// HTTP/2 support. This is because we need to intercept the
		// connection and we can't do that with HTTP/2.
		TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler)),
	}

	return p, nil
}

// ListenAndServe listens for and serves connections. It blocks until the
// context is done or server listening procedure returns an error.
func (p *Proxy) ListenAndServe(ctx context.Context) {
	// NOTE: By having stop channel we can retry opening a server in case
	// the Serve method fails.
	stopCh := make(chan struct{})

	go func() {
		defer func() {
			p.control.Clean()
			close(stopCh)
		}()

		il, err := intercept.NewListener(
			p.srv.Addr,
			p.limiter,
			p.control,
		)
		if err != nil {
			p.silentError(err, "creating listener")

			return
		}

		err = p.srv.Serve(il)
		if err != nil {
			p.silentError(err, "listening and serving")
		}
	}()

	select {
	case <-stopCh:
	case <-ctx.Done():
		closureCtx, closureCancel := context.WithTimeout(context.Background(), _closeTimeout)
		defer closureCancel()

		err := p.srv.Shutdown(closureCtx) //nolint: contextcheck // we cannot use base context here as it is already cancelled and we want to give time for a shutdown.
		if err != nil {
			p.silentError(err, "shutting down server")
		}

		<-stopCh
	}
}

// authHandler checks if the provided proxy credentials are valid. In case
// they are invalid, the proxy responds with a 407 status code and a
// Proxy-Authenticate header.
func (p *Proxy) authHandler(w http.ResponseWriter, r *http.Request) {
	// NOTE: In order for us to be able to respond with a proper error
	// message, we need to allow the initial request data to be read.
	if p.control.HasRemove(r.RemoteAddr) {
		http.Error(w, "bytes limit exceeded", http.StatusRequestEntityTooLarge)
		return
	}

	if !p.auth(r.Header.Get("Proxy-Authorization")) {
		w.Header().Set("Proxy-Authenticate", "Basic")
		w.WriteHeader(http.StatusProxyAuthRequired)

		return
	}

	p.recordHandler(w, r)
}

// recordHandler creates a new request record and publishes it to the
// recorder.
func (p *Proxy) recordHandler(w http.ResponseWriter, r *http.Request) {
	// NOTE: We publish a request record before handling the request.
	// This could be done in a separate goroutine to avoid blocking the
	// request handling as we don't know what will be done in the publish
	// method. However, in that case we should track the number
	// of spinned go routines and keep a limit on them. Other solution
	// could be to use a buffered channel communication. In case we publish
	// directly to a message broker, we should be able to avoid these
	// problems, except for the error handling.
	if err := p.rec.Handle(request.NewRecord(r.Host)); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	p.requestHandler(w, r)
}

// requestHandler determines how the request should be proxied.
func (p *Proxy) requestHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithDeadline(
		r.Context(),
		time.Now().Add(_connectionTimeout),
	)
	defer cancel()

	r = r.WithContext(ctx)

	if r.Method == http.MethodConnect {
		p.tunnelingHandler(w, r)
		return
	}

	p.httpHandler(w, r)
}

// auth handles proxy authentication checking.
func (p *Proxy) auth(header string) bool {
	bkey := strings.SplitN(header, " ", 2)
	if bkey[0] != "Basic" {
		return false
	}

	key, err := base64.StdEncoding.DecodeString(bkey[1])
	if err != nil {
		p.log.Debug("decoding basic auth", slog.String("error", err.Error()))

		return false
	}

	parts := strings.SplitN(string(key), ":", 2)
	if len(parts) != 2 || p.cfg.Auth.Username != parts[0] || p.cfg.Auth.Password != parts[1] {
		return false
	}

	return true
}

// silentError returns true if the error can be silenced.
func (p *Proxy) silentError(err error, msg string) {
	fn := p.log.Error

	if errors.Is(err, os.ErrDeadlineExceeded) ||
		errors.Is(err, net.ErrClosed) ||
		errors.Is(err, enforce.ErrLimitExceeded) ||
		errors.Is(err, http.ErrServerClosed) {
		fn = p.log.Debug
	}

	fn(msg, slog.String("error", err.Error()))
}

// Recorder should be used to record proxy requests.
type Recorder interface {
	// Handle should handle a new record.
	Handle(rec request.Record) error
}

// DB is an interface for a database communication.
type DB interface {
	enforce.DB
}
