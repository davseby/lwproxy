package main

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/davseby/lwproxy/internal/server"
	"golang.org/x/exp/slog"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	defer logger.Info("application shutdown")

	stop, err := startServices(logger)
	if err != nil {
		logger.With("error", err).
			Error("starting services")
		return
	}
	defer stop()

	trapInstance(logger)
}

// startServices starts the application services.
func startServices(logger *slog.Logger) (func(), error) {
	ctx, cancel := context.WithCancel(context.Background())

	server, err := server.NewServer(logger)
	if err != nil {
		cancel()
		return nil, err
	}

	var wg sync.WaitGroup

	wg.Add(1)

	go func() {
		defer wg.Done()

		for ctx.Err() == nil {
			err := server.ListenAndServe(ctx)
			if err != nil {
				logger.With(err).
					Error("starting server")
			}
		}
	}()

	return func() {
		cancel()
		wg.Wait()
	}, nil
}

// trapInstance blocks until a termination signal is received.
func trapInstance(logger *slog.Logger) {
	terminationCh := make(chan os.Signal, 1)

	signal.Notify(terminationCh, syscall.SIGINT, syscall.SIGTERM)

	<-terminationCh

	logger.Info("initiating shutdown")
}
