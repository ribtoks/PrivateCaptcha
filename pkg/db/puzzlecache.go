package db

import (
	"context"
	"time"

	"github.com/maypok86/otter/v2"
)

type puzzleCache struct {
	store *otter.Cache[uint64, uint32]
}

func newPuzzleCache(expiryTTL time.Duration) *puzzleCache {
	const maxSize = 500_000
	const initialSize = 1_000

	return &puzzleCache{
		store: otter.Must(&otter.Options[uint64, uint32]{
			MaximumSize:      maxSize,
			InitialCapacity:  initialSize,
			ExpiryCalculator: otter.ExpiryAccessing[uint64, uint32](expiryTTL),
		}),
	}
}

func (pc *puzzleCache) CheckCount(ctx context.Context, key uint64, maxCount uint32) bool {
	if count, ok := pc.store.GetIfPresent(key); ok {
		return count < maxCount
	}

	return true
}

func puzzleCacheRemapInc(oldValue uint32, found bool) (newValue uint32, op otter.ComputeOp) {
	if !found {
		return 1, otter.WriteOp
	}

	return oldValue + 1, otter.WriteOp
}

func (pc *puzzleCache) Inc(ctx context.Context, key uint64, ttl time.Duration) uint32 {
	value, _ := pc.store.Compute(key, puzzleCacheRemapInc)

	if value == 1 {
		pc.store.SetExpiresAfter(key, ttl)
	}

	return value
}
