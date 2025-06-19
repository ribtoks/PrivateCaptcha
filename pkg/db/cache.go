package db

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/common"
	"github.com/maypok86/otter/v2"
	"github.com/maypok86/otter/v2/stats"
)

var (
	ErrNegativeCacheHit = errors.New("negative hit")
	ErrCacheMiss        = errors.New("cache miss")
	ErrSetMissing       = errors.New("cannot set missing value directly")
)

type memcache[TKey comparable, TValue comparable] struct {
	store   *otter.Cache[TKey, TValue]
	counter *stats.Counter
	// TODO: Evaluate storing negative cache separately from the main one
	// with much smaller max size
	missingValue TValue
	missingTTL   time.Duration
}

func NewMemoryCache[TKey comparable, TValue comparable](maxCacheSize int, missingValue TValue, expiryTTL, refreshTTL, missingTTL time.Duration) (*memcache[TKey, TValue], error) {
	counter := stats.NewCounter()
	store, err := otter.New(&otter.Options[TKey, TValue]{
		MaximumSize:       maxCacheSize,
		ExpiryCalculator:  otter.ExpiryAccessing[TKey, TValue](expiryTTL),
		RefreshCalculator: otter.RefreshWriting[TKey, TValue](refreshTTL),
		StatsRecorder:     counter,
	})

	if err != nil {
		return nil, err
	}

	return &memcache[TKey, TValue]{
		store:        store,
		counter:      counter,
		missingValue: missingValue,
		missingTTL:   missingTTL,
	}, nil
}

var _ common.Cache[int, any] = (*memcache[int, any])(nil)

func (c *memcache[TKey, TValue]) Missing() TValue {
	return c.missingValue
}

func (c *memcache[TKey, TValue]) HitRatio() float64 {
	return c.counter.Snapshot().HitRatio()
}

func (c *memcache[TKey, TValue]) Get(ctx context.Context, key TKey) (TValue, error) {
	data, found := c.store.GetIfPresent(key)
	if !found {
		slog.Log(ctx, common.LevelTrace, "Item not found in memory cache", "key", key)
		var zero TValue
		return zero, ErrCacheMiss
	}

	if data == c.missingValue {
		slog.Log(ctx, common.LevelTrace, "Item set as missing in memory cache", "key", key)
		var zero TValue
		return zero, ErrNegativeCacheHit
	}

	slog.Log(ctx, common.LevelTrace, "Found item in memory cache", "key", key)

	return data, nil
}

func (c *memcache[TKey, TValue]) GetEx(ctx context.Context, key TKey, loader func(context.Context, TKey) (TValue, error)) (TValue, error) {
	data, err := c.store.Get(ctx, key, otter.LoaderFunc[TKey, TValue](loader))
	if err != nil {
		if errors.Is(err, otter.ErrNotFound) {
			slog.Log(ctx, common.LevelTrace, "Item not found in memory cache", "key", key)

			var zero TValue
			return zero, ErrCacheMiss
		}

		slog.ErrorContext(ctx, "Failed to get item from memory cache", "key", key, common.ErrAttr(err))

		return data, err
	}

	if data == c.missingValue {
		// we force-set TTL as it means loader function returned missing value, in contrast to using function SetMission()
		c.store.SetExpiresAfter(key, c.missingTTL)
		slog.Log(ctx, common.LevelTrace, "Item set as missing in memory cache", "key", key)
		var zero TValue
		return zero, ErrNegativeCacheHit
	}

	slog.Log(ctx, common.LevelTrace, "Found item in memory cache", "key", key)

	return data, nil
}

func (c *memcache[TKey, TValue]) SetMissing(ctx context.Context, key TKey) error {
	c.store.Set(key, c.missingValue)
	c.store.SetExpiresAfter(key, c.missingTTL)

	slog.Log(ctx, common.LevelTrace, "Set item as missing in memory cache", "key", key)

	return nil
}

func (c *memcache[TKey, TValue]) Set(ctx context.Context, key TKey, t TValue) error {
	if t == c.missingValue {
		return ErrSetMissing
	}

	c.store.Set(key, t)

	slog.Log(ctx, common.LevelTrace, "Saved item to memory cache", "key", key)

	return nil
}

func (c *memcache[TKey, TValue]) SetWithTTL(ctx context.Context, key TKey, t TValue, ttl time.Duration) error {
	if t == c.missingValue {
		return ErrSetMissing
	}

	c.store.Set(key, t)
	c.store.SetExpiresAfter(key, ttl)

	slog.Log(ctx, common.LevelTrace, "Saved item to memory cache", "key", key, "ttl", ttl)

	return nil
}

func (c *memcache[TKey, TValue]) Delete(ctx context.Context, key TKey) error {
	_, found := c.store.Invalidate(key)

	slog.Log(ctx, common.LevelTrace, "Deleted item from memory cache", "key", key, "found", found)

	return nil
}

type cacheKeyPrefix byte

const (
	userCacheKeyPrefix cacheKeyPrefix = iota
	apiKeyCacheKeyPrefix
	orgCacheKeyPrefix
	orgPropertiesCacheKeyPrefix
	propertyByIDCacheKeyPrefix
	propertyBySitekeyCacheKeyPrefix
	userOrgsCacheKeyPrefix
	orgUsersCacheKeyPrefix
	userAPIKeysCacheKeyPrefix
	subscriptionCacheKeyPrefix
	notificationCacheKeyPrefix
)

// it's a "union" type which is better than doing string concatenation as before
type CacheKey struct {
	Prefix   cacheKeyPrefix
	IntValue int32
	StrValue string
}

func (ck CacheKey) String() string {
	var prefix string
	switch ck.Prefix {
	case userCacheKeyPrefix:
		prefix = "user/"
	case apiKeyCacheKeyPrefix:
		prefix = "apikey/"
	case orgCacheKeyPrefix:
		prefix = "org/"
	case orgPropertiesCacheKeyPrefix:
		prefix = "orgProperties/"
	case propertyByIDCacheKeyPrefix:
		prefix = "propID/"
	case propertyBySitekeyCacheKeyPrefix:
		prefix = "propSitekey/"
	case userOrgsCacheKeyPrefix:
		prefix = "userOrgs/"
	case orgUsersCacheKeyPrefix:
		prefix = "orgUsers/"
	case userAPIKeysCacheKeyPrefix:
		prefix = "userApiKeys/"
	case subscriptionCacheKeyPrefix:
		prefix = "subscr/"
	case notificationCacheKeyPrefix:
		prefix = "notif/"
	}

	if len(ck.StrValue) != 0 {
		return prefix + ck.StrValue
	}

	return prefix + strconv.Itoa(int(ck.IntValue))
}

func (ck CacheKey) LogValue() slog.Value {
	return slog.StringValue(ck.String())
}

func int32CacheKey(prefix cacheKeyPrefix, value int32) CacheKey {
	return CacheKey{
		Prefix:   prefix,
		IntValue: value,
		StrValue: "",
	}
}

func stringCacheKey(prefix cacheKeyPrefix, value string) CacheKey {
	return CacheKey{
		Prefix:   prefix,
		IntValue: 0,
		StrValue: value,
	}
}

func userCacheKey(id int32) CacheKey     { return int32CacheKey(userCacheKeyPrefix, id) }
func APIKeyCacheKey(str string) CacheKey { return stringCacheKey(apiKeyCacheKeyPrefix, str) }
func orgCacheKey(orgID int32) CacheKey   { return int32CacheKey(orgCacheKeyPrefix, orgID) }
func orgPropertiesCacheKey(orgID int32) CacheKey {
	return int32CacheKey(orgPropertiesCacheKeyPrefix, orgID)
}
func propertyByIDCacheKey(propID int32) CacheKey {
	return int32CacheKey(propertyByIDCacheKeyPrefix, propID)
}
func PropertyBySitekeyCacheKey(sitekey string) CacheKey {
	return stringCacheKey(propertyBySitekeyCacheKeyPrefix, sitekey)
}
func userOrgsCacheKey(userID int32) CacheKey { return int32CacheKey(userOrgsCacheKeyPrefix, userID) }
func orgUsersCacheKey(orgID int32) CacheKey  { return int32CacheKey(orgUsersCacheKeyPrefix, orgID) }
func userAPIKeysCacheKey(userID int32) CacheKey {
	return int32CacheKey(userAPIKeysCacheKeyPrefix, userID)
}
func subscriptionCacheKey(sID int32) CacheKey { return int32CacheKey(subscriptionCacheKeyPrefix, sID) }
func notificationCacheKey(ID int32) CacheKey  { return int32CacheKey(notificationCacheKeyPrefix, ID) }
