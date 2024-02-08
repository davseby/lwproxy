package proxy

import (
	"context"
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/davseby/lwproxy/internal/proxy/enforce"
	"github.com/davseby/lwproxy/internal/proxy/intercept"
	"github.com/davseby/lwproxy/internal/request"
	"golang.org/x/exp/slog"
)

const (
	// _closeTimeout is the timeout for closing the proxy.
	_closeTimeout = 5 * time.Second

	// _targetDialTimeout is the timeout for dialing the target.
	_targetDialTimeout = 10 * time.Second

	// _connectionDeadline is the deadline for a connection.
	_connectionDeadline = 2 * time.Hour
)

// Proxy is a proxy server.
type Proxy struct {
	log *slog.Logger

	srv *http.Server

	rp Recorder
	bl intercept.BytesLimiter

	cfg ProxyConfig
	cp  *intercept.ControlPoint
}

// ProxyConfig holds the settings for the proxy server.
type ProxyConfig struct {
	// Addr is the address to listen on.
	Addr string `default:":8081"`

	// MaxBytes is the maximum amount of bytes that can be used.
	MaxBytes int64 `default:"2000000000"`

	// Username is the username for basic authentication.
	Username string `default:"admin"`

	// Password is the password for basic authentication.
	Password string `default:"admin"`
}

// NewProxy creates a new proxy server.
func NewProxy(
	log *slog.Logger,
	rp Recorder,
	db DB,
	cfg ProxyConfig,
) (*Proxy, error) {
	var bl intercept.BytesLimiter = enforce.NewNoopBytesLimiter()

	if cfg.MaxBytes > 0 {
		bl = enforce.NewBytesLimiter(log, db, cfg.MaxBytes)
	}

	p := &Proxy{
		log: log.With("job", "proxy"),
		rp:  rp,
		cfg: cfg,
		bl:  bl,
		cp:  intercept.NewControlPoint(),
	}

	p.srv = &http.Server{
		Addr:    cfg.Addr,
		Handler: p,
	}

	return p, nil
}

// ListenAndServe listens for and serves connections. It blocks until the
// context is done or server listening procedure returns an error.
func (p *Proxy) ListenAndServe(ctx context.Context) {
	// NOTE: By having stop channel we can retry opening a server in case
	// the ListenAndServe method fails.
	stopCh := make(chan struct{})

	go func() {
		defer func() {
			p.cp.Clean()
			close(stopCh)
		}()

		il, err := intercept.NewListener(
			"tcp",
			p.srv.Addr,
			p.bl,
			p.cp,
		)
		if err != nil {
			p.log.With("error", err).
				Error("creating listener")

			return
		}

		err = p.srv.Serve(il)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			p.log.With("error", err).
				Error("listening and serving")
		}

		time.Sleep(time.Second)
	}()

	select {
	case <-stopCh:
	case <-ctx.Done():
		closureCtx, closureCancel := context.WithTimeout(context.Background(), _closeTimeout)
		defer closureCancel()

		err := p.srv.Shutdown(closureCtx)
		if err != nil {
			p.log.With("error", err).
				Error("shutting down server")
		}

		<-stopCh
	}
}

// ServeHTTP serves HTTP requests.
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !p.handleAuth(w, r) {
		return
	}

	// NOTE: We publish a request record before handling the request.
	// This could be done in a separate goroutine to avoid blocking the
	// request handling as we don't know what will be done in the publish
	// method. However, in that case we should track the number
	// of spinned go routines and keep a limit on them. Other solution
	// could be to use a buffered channel communication. In case we publish
	// directly to a message broker, we should be able to avoid these
	// problems, except for the error handling.
	if err := p.rp.Create(request.NewRecord(r.Host)); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// NOTE: In order for us to to be able to respond with a proper error
	// message, we need to allow the initial request data to be read. This
	// check is critical as if we don't use this, the client will be able
	// to use as much traffic as it wants, without it being logged.
	if p.cp.HasRemove(r.RemoteAddr) {
		http.Error(w, "bytes limit exceeded", http.StatusRequestEntityTooLarge)
		return
	}

	ctx, cancel := context.WithDeadline(
		r.Context(),
		time.Now().Add(_connectionDeadline),
	)
	defer cancel()

	if r.Method == http.MethodConnect {
		p.handleTunneling(w, r.WithContext(ctx))
		return
	}

	p.handleRequest(w, r.WithContext(ctx))
}

// handleAuth handles the basic authentication.
func (p *Proxy) handleAuth(w http.ResponseWriter, r *http.Request) bool {
	key := strings.SplitN(r.Header.Get("Proxy-Authorization"), " ", 2)
	if key[0] == "Basic" {
		key, err := base64.StdEncoding.DecodeString(key[1])
		if err != nil {
			http.Error(w, "authorization failed", http.StatusUnauthorized)

			return false
		}

		parts := strings.SplitN(string(key), ":", 2)
		if len(parts) != 2 || p.cfg.Username != parts[0] || p.cfg.Password != parts[1] {
			http.Error(w, "authorization failed", http.StatusUnauthorized)

			return false
		}

		return true
	}

	resp := http.Response{
		StatusCode: http.StatusProxyAuthRequired,
		Header: http.Header{
			"Proxy-Authenticate": []string{
				"Basic realm=\"lwproxy\"",
			},
		},
		ProtoMajor: 1,
		ProtoMinor: 1,
	}

	err := resp.Write(w)
	if err != nil {
		p.log.With("error", err).
			Error("writing authentication response")
	}

	return false
}

// silentError returns true if the error can be silenced.
func silentError(err error) bool {
	return errors.Is(err, context.Canceled) ||
		errors.Is(err, os.ErrDeadlineExceeded) ||
		errors.Is(err, net.ErrClosed) ||
		errors.Is(err, enforce.ErrLimitExceeded)
}

// Recorder should be used to record proxy requests.
type Recorder interface {
	// Create should create a new record.
	Create(rec request.Record) error
}

// DB is an interface for a database communication.
type DB interface {
	enforce.DB
}
