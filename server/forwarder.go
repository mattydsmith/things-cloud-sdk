package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/arthursoares/things-cloud-sdk/sync"
)

// ForwardTarget configures where the forwarder POSTs events.
type ForwardTarget struct {
	URL          string        // base URL, e.g. https://things-plus.fly.dev
	AuthToken    string        // bearer token for things-plus
	BatchSize    int           // rows per change_log read; default 200
	HTTPClient   *http.Client  // optional; defaults to http.DefaultClient with a 30s timeout
	IdleInterval time.Duration // sleep when caught up; default 60s
}

// ForwardReport is the per-tick result, returned for logging.
type ForwardReport struct {
	Forwarded int
	Cursor    int64
}

const (
	defaultBatchSize    = 200
	defaultIdleInterval = 60 * time.Second
	httpRequestTimeout  = 30 * time.Second
	eventsPath          = "/events"
)

// forwardOnce drains change_log past the forward cursor in batches, POSTing
// each row to target.URL+/events. The cursor advances after each successful
// POST. Returns on the first POST or DB error, or when caught up.
func forwardOnce(ctx context.Context, syncer *sync.Syncer, target ForwardTarget) (ForwardReport, error) {
	batchSize := target.BatchSize
	if batchSize <= 0 {
		batchSize = defaultBatchSize
	}
	httpClient := target.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: httpRequestTimeout}
	}

	cursor, err := syncer.GetForwardCursor()
	if err != nil {
		return ForwardReport{}, fmt.Errorf("get forward cursor: %w", err)
	}

	var forwarded int

	for {
		if err := ctx.Err(); err != nil {
			return ForwardReport{Forwarded: forwarded, Cursor: cursor}, err
		}

		rows, err := syncer.RawChangesSinceID(cursor, batchSize)
		if err != nil {
			return ForwardReport{Forwarded: forwarded, Cursor: cursor}, fmt.Errorf("read change_log: %w", err)
		}
		if len(rows) == 0 {
			return ForwardReport{Forwarded: forwarded, Cursor: cursor}, nil
		}

		for _, row := range rows {
			if err := ctx.Err(); err != nil {
				return ForwardReport{Forwarded: forwarded, Cursor: cursor}, err
			}
			if err := postRow(ctx, httpClient, target, row); err != nil {
				return ForwardReport{Forwarded: forwarded, Cursor: cursor}, fmt.Errorf("post change_log id=%d: %w", row.ID, err)
			}
			// If postRow succeeded but SetForwardCursor fails, the next forwardOnce
			// will re-POST this row. things-plus accepts duplicates (no source-side
			// dedup); see plan TD-1. Acceptable at our volume.
			if err := syncer.SetForwardCursor(row.ID); err != nil {
				return ForwardReport{Forwarded: forwarded, Cursor: cursor}, fmt.Errorf("advance cursor to %d: %w", row.ID, err)
			}
			cursor = row.ID
			forwarded++
		}

		// Stop when this batch was the last (fewer than batchSize means no more rows).
		if len(rows) < batchSize {
			return ForwardReport{Forwarded: forwarded, Cursor: cursor}, nil
		}
	}
}

// postRow POSTs a single change_log row as an EventCreate to things-plus.
func postRow(ctx context.Context, client *http.Client, target ForwardTarget, row sync.ChangeLogRow) error {
	body, err := buildEventBody(row)
	if err != nil {
		return fmt.Errorf("build body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target.URL+eventsPath, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+target.AuthToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(respBody))
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

// buildEventBody maps a change_log row to the things-plus EventCreate shape.
// Lifts entity fields and serverIndex into payload for downstream consumers
// (the daily Sonnet pass) to read without re-parsing the source JSON.
func buildEventBody(row sync.ChangeLogRow) ([]byte, error) {
	payload := map[string]any{
		"entityType":  row.EntityType,
		"entityUUID":  row.EntityUUID,
		"serverIndex": row.ServerIndex,
		"changeLogId": row.ID,
	}
	if row.Payload != "" {
		var src any
		if err := json.Unmarshal([]byte(row.Payload), &src); err == nil {
			payload["source"] = src
		} else {
			payload["sourceRaw"] = row.Payload
		}
	}

	return json.Marshal(map[string]any{
		"source":    "things",
		"kind":      row.ChangeType,
		"timestamp": row.SyncedAt.UTC().Format(time.RFC3339Nano),
		"payload":   payload,
	})
}

// RunForwardLoop runs forwardOnce on a ticker. Returns when ctx is cancelled.
// syncFn is called once per tick before forwarding to ensure change_log is
// fresh; pass nil to skip (e.g. in tests where the syncer has no live client).
func RunForwardLoop(ctx context.Context, syncer *sync.Syncer, target ForwardTarget, syncFn func() error) {
	interval := target.IdleInterval
	if interval <= 0 {
		interval = defaultIdleInterval
	}
	log.Printf("[FORWARD] starting loop: target=%s interval=%s batch=%d", target.URL, interval, target.BatchSize)

	for {
		if syncFn != nil {
			if err := syncFn(); err != nil {
				log.Printf("[FORWARD] sync error: %v", err)
			}
		}

		report, err := forwardOnce(ctx, syncer, target)
		if err != nil {
			if ctx.Err() != nil {
				log.Printf("[FORWARD] loop exiting: %v", ctx.Err())
				return
			}
			log.Printf("[FORWARD] forwardOnce error after %d rows (cursor=%d): %v", report.Forwarded, report.Cursor, err)
		} else {
			log.Printf("[FORWARD] forwarded %d rows; cursor=%d", report.Forwarded, report.Cursor)
		}

		select {
		case <-ctx.Done():
			log.Printf("[FORWARD] loop exiting: %v", ctx.Err())
			return
		case <-time.After(interval):
		}
	}
}
