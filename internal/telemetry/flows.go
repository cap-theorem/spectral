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
)

const flowPushTimeout = time.Second

type FlowExporter struct {
	url    string
	flows  <-chan collector.FlowReport
	client *http.Client
	logger *slog.Logger
}

func NewFlowExporter(cfg ExporterConfig, flows <-chan collector.FlowReport, logger *slog.Logger) *FlowExporter {
	return &FlowExporter{
		url:    strings.TrimRight(cfg.CollectorURL, "/") + collector.FlowsPath,
		flows:  flows,
		client: &http.Client{Timeout: flowPushTimeout},
		logger: logger,
	}
}

func (e *FlowExporter) Run(ctx context.Context) error {
	failing := false
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case r := <-e.flows:
			err := e.push(ctx, r)
			switch {
			case err != nil && !failing:
				e.logger.Warn("flow report failed, dropping flows until it recovers", "err", err)
				failing = true
			case err == nil && failing:
				e.logger.Info("flow reports recovered")
				failing = false
			}
		}
	}
}

func (e *FlowExporter) push(ctx context.Context, r collector.FlowReport) error {
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
