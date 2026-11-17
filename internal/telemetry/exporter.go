package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/cap-theorem/spectral/internal/collector"
	"github.com/cap-theorem/spectral/internal/wire"
)

const (
	minBackoff  = 500 * time.Millisecond
	maxBackoff  = 30 * time.Second
	pushTimeout = 2 * time.Second
)

// Exporter posts the reports the node sends it. On failure it backs off and
// retries the newest report; older ones are dropped.
type Exporter struct {
	url     string
	reports <-chan collector.Report
	client  *http.Client
	logger  *slog.Logger
	seq     uint64
}

type ExporterConfig struct {
	CollectorURL string
}

func NewExporter(cfg ExporterConfig, reports <-chan collector.Report, logger *slog.Logger) *Exporter {
	return &Exporter{
		url:     strings.TrimRight(cfg.CollectorURL, "/") + collector.ReportPath,
		reports: reports,
		client:  &http.Client{Timeout: pushTimeout},
		logger:  logger,
	}
}

func (e *Exporter) Run(ctx context.Context) error {
	e.logger.Info("topology exporter started", "collector", e.url)

	var (
		report  collector.Report
		pending bool
		retry   <-chan time.Time
		backoff = minBackoff
		failing bool
	)

	for {
		select {
		case <-ctx.Done():
			e.leave(report)
			return ctx.Err()
		case report = <-e.reports:
			pending = true
		case <-retry:
			retry = nil
			select {
			case report = <-e.reports:
			default:
			}
		}
		if !pending || retry != nil {
			continue
		}

		if err := e.push(ctx, report); err != nil {
			if !failing {
				e.logger.Warn("topology report failed, backing off", "err", err)
			}
			failing = true
			retry = time.After(backoff)
			backoff = min(2*backoff, maxBackoff)
			continue
		}
		if failing {
			e.logger.Info("topology reports recovered")
		}
		failing = false
		backoff = minBackoff
		pending = false
	}
}

// Best effort: the collector expires the node anyway.
func (e *Exporter) leave(last collector.Report) {
	if last.ID == (wire.NodeID{}) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := e.push(ctx, collector.Report{ID: last.ID, Address: last.Address, Left: true}); err != nil {
		e.logger.Debug("topology leave report failed", "err", err)
	}
}

func (e *Exporter) push(ctx context.Context, r collector.Report) error {
	e.seq++
	r.Seq = e.seq
	body, err := json.Marshal(r)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 300 {
		return fmt.Errorf("collector answered %s", resp.Status)
	}
	return nil
}
