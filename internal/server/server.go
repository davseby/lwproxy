package server

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/exp/slog"
)

// Server is a server.
type Server struct {
	logger *slog.Logger
	server *http.Server
}

// NewServer creates a new server.
func NewServer(logger *slog.Logger) (*Server, error) {
	return &Server{
		server: &http.Server{},
		logger: logger.With("thread", "server"),
	}, nil
}

// ListenAndServe listens for and serves connections.
func (s *Server) ListenAndServe(ctx context.Context) error {
	ln, err := net.Listen("tcp", ":8080")

	if err != nil {
		return err
	}

	var wg sync.WaitGroup

	go func() {
		<-ctx.Done()

		if err := ln.Close(); err != nil {
			s.logger.With("error", err).
				Error("closing listener")
		}
	}()

	for ctx.Err() == nil {
		conn, err := ln.Accept()
		switch {
		case err == nil:
			// OK.
		case errors.Is(err, net.ErrClosed):
			return nil
		default:
			s.logger.With("error", err).
				Error("accepting connection")

			continue
		}

		wg.Add(1)

		go func() {
			defer wg.Done()

			s.handleConnection(ctx, conn)
		}()
	}

	wg.Wait()

	return nil
}

// handleConnection handles a connection.
func (s *Server) handleConnection(ctx context.Context, baseConn net.Conn) {
	targetConn, req, err := dialTarget(baseConn)
	if err != nil {
		s.logger.With("error", err).
			Error("setting up target")

		err := baseConn.Close()
		if err != nil && !errors.Is(err, net.ErrClosed) {
			s.logger.With("error", err).
				Error("closing base connection")
		}

		return
	}

	s.logger.With("request", req.URL.Host).Info("handling connection")

	closeConnections := func() {
		err := targetConn.Close()
		if err != nil && !errors.Is(err, net.ErrClosed) {
			s.logger.With("error", err).
				Error("closing target connection")
		}

		err = baseConn.Close()
		if err != nil && !errors.Is(err, net.ErrClosed) {
			s.logger.With("error", err).
				Error("closing base connection")
		}
	}

	var wg sync.WaitGroup

	wg.Add(1)

	go func() {
		defer wg.Done()

		err := negiotateCommunication(ctx, baseConn, targetConn)
		if err != nil {
			s.logger.With("error", err).
				Error("negiotating communication")
		}

		closeConnections()
	}()

	wg.Add(1)

	go func() {
		defer wg.Done()

		err := negiotateCommunication(ctx, targetConn, baseConn)
		if err != nil {
			s.logger.With("error", err).
				Error("negiotating communication")
		}

		closeConnections()
	}()

	wg.Wait()

	s.logger.With("request", req.URL.Host).Info("connection handled")
}

// dialTarget sets up the target connection.
func dialTarget(baseConn net.Conn) (net.Conn, *http.Request, error) {
	buf := make([]byte, 65535)

	n, err := baseConn.Read(buf)
	if err != nil {
		return nil, nil, fmt.Errorf("reading initial request: %w", err)
	}

	req, err := http.ReadRequest(
		bufio.NewReader(
			bytes.NewBuffer(buf[:n]),
		),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("deriving target: %w", err)
	}

	targetConn, err := net.DialTimeout("tcp", req.URL.Host, time.Second*5)
	if err != nil {
		return nil, nil, fmt.Errorf("dialing target: %w", err)
	}

	rbuf := make([]byte, 65535)

	err = req.Write(bytes.NewBuffer(rbuf))
	if err != nil {
		return nil, nil, fmt.Errorf("writing initial request: %w", err)
	}

	_, err = targetConn.Write(rbuf[:n])
	if err != nil {
		return nil, nil, fmt.Errorf("writing initial request: %w", err)
	}

	return targetConn, req, nil
}

// negiotateCommunication negiotates communication between source and
// destination connections.
func negiotateCommunication(ctx context.Context, src, dst net.Conn) error {
	buf := make([]byte, 65535)

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

		_, err = dst.Write(buf[:n])
		if err != nil {
			return fmt.Errorf("writing to destination: %w", err)
		}
	}

	return nil
}
