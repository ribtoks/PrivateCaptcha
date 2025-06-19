package common

import (
	"context"
	"log/slog"
	"time"
)

func ProcessBatchArray[T any](ctx context.Context, channel <-chan T, delay time.Duration, triggerSize, maxBatchSize int, processor func(context.Context, []T) error) {
	var batch []T
	slog.DebugContext(ctx, "Processing batch", "interval", delay.String())

	for running := true; running; {
		if len(batch) > maxBatchSize {
			slog.ErrorContext(ctx, "Dropping pending batch due to errors", "count", len(batch))
			batch = []T{}
		}

		select {
		case <-ctx.Done():
			running = false

		case item, ok := <-channel:
			if !ok {
				running = false
				break
			}

			batch = append(batch, item)

			if len(batch) >= triggerSize {
				slog.Log(ctx, LevelTrace, "Processing batch", "count", len(batch), "reason", "batch")
				if err := processor(ctx, batch); err == nil {
					batch = []T{}
				}
			}
		case <-time.After(delay):
			if len(batch) > 0 {
				slog.Log(ctx, LevelTrace, "Processing batch", "count", len(batch), "reason", "timeout")
				if err := processor(ctx, batch); err == nil {
					batch = []T{}
				}
			}
		}
	}

	slog.InfoContext(ctx, "Finished processing batch")
}

// as they say, a little copy-paste is better than a little dependency
// it is assumed to be called with such parameters that make uint enough for counting
func ProcessBatchMap[T comparable](ctx context.Context, channel <-chan T, delay time.Duration, triggerSize, maxBatchSize int, processor func(context.Context, map[T]uint) error) {
	batch := make(map[T]uint)
	slog.DebugContext(ctx, "Processing batch", "interval", delay.String())

	for running := true; running; {
		if len(batch) > maxBatchSize {
			slog.ErrorContext(ctx, "Dropping pending batch due to errors", "count", len(batch))
			batch = make(map[T]uint)
		}

		select {
		case <-ctx.Done():
			running = false

		case item, ok := <-channel:
			if !ok {
				running = false
				break
			}

			batch[item]++

			if len(batch) >= triggerSize {
				slog.Log(ctx, LevelTrace, "Processing batch", "count", len(batch), "reason", "batch")
				if err := processor(ctx, batch); err == nil {
					batch = make(map[T]uint)
				}
			}
		case <-time.After(delay):
			if len(batch) > 0 {
				slog.Log(ctx, LevelTrace, "Processing batch", "count", len(batch), "reason", "timeout")
				if err := processor(ctx, batch); err == nil {
					batch = make(map[T]uint)
				}
			}
		}
	}

	slog.InfoContext(ctx, "Finished processing batch")
}
