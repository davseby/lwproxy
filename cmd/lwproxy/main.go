package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/cristalhq/aconfig"
	"github.com/cristalhq/aconfig/aconfigyaml"
	"github.com/davseby/lwproxy/internal/db/memory"
	"github.com/davseby/lwproxy/internal/proxy"
	"github.com/davseby/lwproxy/internal/request/process/stdout"
	"golang.org/x/exp/slog"
)

// _retryTimeout is the timeout for retrying.
const _retryTimeout = 5 * time.Second

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
	var configPath string

	flag.StringVar(&configPath, "config", "config/.env.config.yaml", "path to the configuration file")
	flag.Parse()

	var cfg Config

	err := aconfig.LoaderFor(&cfg, aconfig.Config{
		SkipEnv:   true,
		SkipFlags: true,
		Files: []string{
			configPath,
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

			contextRetry(ctx)
		}
	}()

	return func() {
		cancel()
		wg.Wait()
	}, nil
}

// trapInstance blocks until a termination signal is received.
// NOTE: For a mono-repo setup we could move the trapInstance and
// contextRetry functions to a separate internal package.
func trapInstance(logger *slog.Logger) {
	terminationCh := make(chan os.Signal, 1)

	signal.Notify(terminationCh, syscall.SIGINT, syscall.SIGTERM)

	<-terminationCh

	logger.Info("initiating shutdown")
}

// contextRetry waits for the context to be done or the timeout to be reached.
func contextRetry(ctx context.Context) {
	tc := time.NewTimer(_retryTimeout)
	defer func() {
		tc.Stop()

		// This ensures that on context cancellation we drain the
		// channel in case it is not empty.
		// See: https://github.com/golang/go/issues/27169
		select {
		case <-tc.C:
		default:
		}
	}()

	select {
	case <-tc.C:
	case <-ctx.Done():
	}
}
