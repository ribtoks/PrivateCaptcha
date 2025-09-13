package common

import (
	"context"
	"net/http"
	"time"
)

// this is an exact copy of otter's Loader
type CacheLoader[K comparable, V any] interface {
	Load(ctx context.Context, key K) (V, error)
	Reload(ctx context.Context, key K, oldValue V) (V, error)
}

type Cache[TKey comparable, TValue any] interface {
	Get(ctx context.Context, key TKey) (TValue, error)
	GetEx(ctx context.Context, key TKey, loader CacheLoader[TKey, TValue]) (TValue, error)
	SetMissing(ctx context.Context, key TKey) error
	Set(ctx context.Context, key TKey, t TValue) error
	SetWithTTL(ctx context.Context, key TKey, t TValue, ttl time.Duration) error
	Delete(ctx context.Context, key TKey) error
	Missing() TValue
	HitRatio() float64
}

type SessionStore interface {
	Start(ctx context.Context, interval time.Duration)
	Init(ctx context.Context, session *Session) error
	Read(ctx context.Context, sid string) (*Session, error)
	Update(session *Session) error
	Destroy(ctx context.Context, sid string) error
	GC(ctx context.Context, d time.Duration)
}

type ConfigItem interface {
	Key() ConfigKey
	Value() string
}

type ConfigStore interface {
	Get(key ConfigKey) ConfigItem
	Update(ctx context.Context)
}

type TimeSeriesStore interface {
	Ping(ctx context.Context) error
	WriteAccessLogBatch(ctx context.Context, records []*AccessRecord) error
	WriteVerifyLogBatch(ctx context.Context, records []*VerifyRecord) error
	RetrievePropertyStatsSince(ctx context.Context, r *BackfillRequest, from time.Time) ([]*TimeCount, error)
	RetrieveAccountStats(ctx context.Context, userID int32, from time.Time) ([]*TimeCount, error)
	RetrievePropertyStatsByPeriod(ctx context.Context, orgID, propertyID int32, period TimePeriod) ([]*TimePeriodStat, error)
	RetrieveRecentTopProperties(ctx context.Context, limit int) (map[int32]uint, error)
	DeletePropertiesData(ctx context.Context, propertyIDs []int32) error
	DeleteOrganizationsData(ctx context.Context, orgIDs []int32) error
	DeleteUsersData(ctx context.Context, userIDs []int32) error
}

type PlatformMetrics interface {
	ObserveHealth(postgres, clickhouse bool)
	ObserveCacheHitRatio(ratio float64)
}

type APIMetrics interface {
	Handler(h http.Handler) http.Handler
	ObservePuzzleCreated(userID int32)
	ObservePuzzleVerified(userID int32, result string, isStub bool)
}

type PortalMetrics interface {
	HandlerIDFunc(handlerIDFunc func() string) func(http.Handler) http.Handler
}
