package db

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/common"
)

type StaticCache[TKey comparable, TValue comparable] struct {
	cache        map[TKey]TValue
	mux          sync.RWMutex
	upperBound   int
	lowerBound   int
	missingValue TValue
}

func NewStaticCache[TKey comparable, TValue comparable](capacity int, missingValue TValue) *StaticCache[TKey, TValue] {
	return &StaticCache[TKey, TValue]{
		cache:        make(map[TKey]TValue),
		upperBound:   capacity,
		lowerBound:   capacity/2 + capacity/4,
		missingValue: missingValue,
	}
}

func (c *StaticCache[TKey, TValue]) HitRatio() float64 {
	// unsupported
	return 0.0
}

func (c *StaticCache[TKey, TValue]) Missing() TValue {
	return c.missingValue
}

func (c *StaticCache[TKey, TValue]) Get(ctx context.Context, key TKey) (TValue, error) {
	c.mux.RLock()
	defer c.mux.RUnlock()

	if item, ok := c.cache[key]; ok {
		if item == c.missingValue {
			return c.missingValue, ErrNegativeCacheHit
		}

		return item, nil
	} else {
		return c.missingValue, ErrCacheMiss
	}
}

func (c *StaticCache[TKey, TValue]) GetEx(ctx context.Context, key TKey, loader func(context.Context, TKey) (TValue, error)) (TValue, error) {
	c.mux.Lock()
	defer c.mux.Unlock()

	var err error
	item, ok := c.cache[key]
	if !ok {
		if item, err = loader(ctx, key); err == nil {
			c.cache[key] = item
		} else {
			slog.ErrorContext(ctx, "Failed to load the value", "key", key, common.ErrAttr(err))
			return c.missingValue, ErrCacheMiss
		}
	}

	if item == c.missingValue {
		return c.missingValue, ErrNegativeCacheHit
	}

	return item, nil
}

func (c *StaticCache[TKey, TValue]) SetMissing(ctx context.Context, key TKey) error {
	c.mux.Lock()
	defer c.mux.Unlock()

	c.cache[key] = c.missingValue
	return nil
}

func (c *StaticCache[TKey, TValue]) compressUnsafe() {
	for k := range c.cache {
		if len(c.cache) <= c.lowerBound {
			break
		}

		delete(c.cache, k)
	}
}

func (c *StaticCache[TKey, TValue]) Set(ctx context.Context, key TKey, t TValue) error {
	c.mux.Lock()
	defer c.mux.Unlock()

	if len(c.cache) >= c.upperBound {
		c.compressUnsafe()
	}

	c.cache[key] = t
	return nil
}

func (c *StaticCache[TKey, TValue]) SetWithTTL(ctx context.Context, key TKey, t TValue, _ time.Duration) error {
	// ttl is not supported here
	return c.Set(ctx, key, t)
}

func (c *StaticCache[TKey, TValue]) Delete(ctx context.Context, key TKey) error {
	c.mux.Lock()
	defer c.mux.Unlock()

	delete(c.cache, key)
	return nil
}
