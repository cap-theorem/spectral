package daemon

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/cap-theorem/spectral/internal/api"
	"github.com/cap-theorem/spectral/internal/config"
	"github.com/cap-theorem/spectral/internal/node"
	"github.com/golang-cz/devslog"
	"golang.org/x/sync/errgroup"
)

// TODO: Move this to logger package
func newLogger(logFormat string) *slog.Logger {
	// Makes a logger which can output in text for humans and json for containers.
	slogOptions := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}

	var handler slog.Handler

	switch logFormat {
	case "json":
		handler = slog.NewJSONHandler(os.Stdout, slogOptions)
	default:
		handler = devslog.NewHandler(os.Stdout, &devslog.Options{
			HandlerOptions: slogOptions,
			SortKeys:       true,
			DebugColor:     devslog.Blue,
			InfoColor:      devslog.Green,
			WarnColor:      devslog.Yellow,
			ErrorColor:     devslog.Red,
		})
	}

	return slog.New(handler)
}

func Run(cfg config.Config) error {
	sigCtx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	nodeCtx, stopNode := context.WithCancel(context.Background())

	logger := newLogger(cfg.LogFormat)

	node := node.NewActor(
		logger.With("component", "node"),
	)
	api := api.NewAPI(
		node,
		logger.With("component", "api"),
	)

	g, stopping := errgroup.WithContext(sigCtx)

	g.Go(supervise("node", stopping, func() error {
		node.Run(nodeCtx)
		return nil
	}))
	g.Go(supervise("api", stopping, api.Serve))

	var shutdownErr error
	g.Go(func() error {
		<-stopping.Done()

		stop()

		ctx, cancel := context.WithTimeout(
			context.Background(),
			cfg.ShutdownTimeout,
		)
		defer cancel()

		if err := api.Shutdown(ctx); err != nil {
			shutdownErr = errors.Join(
				shutdownErr,
				fmt.Errorf("api shutdown: %w", err),
				api.Close(),
			)
		}

		stopNode()

		return nil
	})

	runErr := g.Wait()

	return errors.Join(runErr, shutdownErr)
}

func supervise(
	name string,
	stopping context.Context,
	run func() error,
) func() error {
	return func() error {
		err := run()

		if stopping.Err() != nil {
			switch {
			case err == nil, errors.Is(err, context.Canceled), errors.Is(err, http.ErrServerClosed):
				return nil
			}
		}

		if err == nil {
			return fmt.Errorf("%s exited unexpectedly", name)
		}

		return fmt.Errorf("%s: %w", name, err)
	}
}
