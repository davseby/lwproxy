package main

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/davseby/lwproxy/internal/request/storage/memory"
	"github.com/davseby/lwproxy/internal/server"
	"github.com/davseby/lwproxy/internal/server/control"
	"golang.org/x/exp/slog"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	defer log.Info("application shutdown")

	stop, err := startServices(log)
	if err != nil {
		log.With("error", err).
			Error("starting services")
		return
	}
	defer stop()

	trapInstance(log)
}

// startServices starts the application services.
func startServices(log *slog.Logger) (func(), error) {
	ctx, cancel := context.WithCancel(context.Background())

	server, err := server.NewServer(
		log,
		control.NewLimiter(log, 1<<15),
		memory.NewHub(log),
	)
	if err != nil {
		cancel()
		return nil, err
	}

	var wg sync.WaitGroup

	wg.Add(1)

	go func() {
		defer wg.Done()

		for ctx.Err() == nil {
			server.ListenAndServe(ctx)
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
