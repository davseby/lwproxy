package main

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/cristalhq/aconfig"
	"github.com/cristalhq/aconfig/aconfigyaml"
	"github.com/davseby/lwproxy/internal/db/memory"
	"github.com/davseby/lwproxy/internal/proxy"
	"github.com/davseby/lwproxy/internal/request/process/stdout"
	"golang.org/x/exp/slog"
)

// Config is the application configuration.
type Config struct {
	// Proxy is the proxy server configuration.
	Proxy proxy.Config

	// Log is the logging configuration.
	Log struct {
		// Level is the logging level.
		Level slog.Level `default:"info"`
	}
}

func main() {
	var cfg Config

	err := aconfig.LoaderFor(&cfg, aconfig.Config{
		SkipEnv:   true,
		SkipFlags: true,
		Files: []string{
			"../../config/.env.config.yaml",
		},
		FileDecoders: map[string]aconfig.FileDecoder{
			".yaml": aconfigyaml.New(),
		},
	}).Load()
	if err != nil {
		slog.Default().Error("loading configuration", slog.String("error", err.Error()))

		return
	}

	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.Log.Level,
	}))
	defer log.Info("application shutdown")

	stop, err := startServices(log, cfg)
	if err != nil {
		log.Error("starting services", slog.String("error", err.Error()))
		return
	}

	defer stop()

	trapInstance(log)
}

// startServices starts the application services.
func startServices(log *slog.Logger, cfg Config) (func(), error) {
	ctx, cancel := context.WithCancel(context.Background())

	server, err := proxy.NewProxy(
		log,
		stdout.NewProcessor(log),
		memory.NewDB(),
		cfg.Proxy,
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
