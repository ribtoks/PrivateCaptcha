package db

import (
	"context"
	"encoding/hex"
	"log/slog"
	"strings"
	"time"

	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/common"
	dbgen "github.com/PrivateCaptcha/PrivateCaptcha/pkg/db/generated"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	SitekeyLen   = 32
	APIKeyPrefix = "pc_"
	SecretLen    = len(APIKeyPrefix) + SitekeyLen
)

var (
	invalidUUID = pgtype.UUID{Valid: false}
)

func IsInternalSubscription(source dbgen.SubscriptionSource) bool {
	switch source {
	case dbgen.SubscriptionSourceExternal:
		return false
	default:
		return true
	}
}

func Text(text string) pgtype.Text {
	return pgtype.Text{
		String: text,
		Valid:  true,
	}
}

func Int(i int32) pgtype.Int4 {
	return pgtype.Int4{Int32: i, Valid: true}
}

func Int2(i int16) pgtype.Int2 {
	return pgtype.Int2{Int16: i, Valid: true}
}

func Bool(b bool) pgtype.Bool {
	return pgtype.Bool{
		Bool:  b,
		Valid: true,
	}
}

func Timestampz(t time.Time) pgtype.Timestamptz {
	if t.IsZero() {
		return pgtype.Timestamptz{Valid: false}
	}

	return pgtype.Timestamptz{
		Time:             t,
		InfinityModifier: pgtype.Finite,
		Valid:            true,
	}
}

func Date(t time.Time) pgtype.Date {
	return pgtype.Date{
		Time:             t,
		InfinityModifier: pgtype.Finite,
		Valid:            true,
	}
}

func UUIDToSiteKey(uuid pgtype.UUID) string {
	if !uuid.Valid {
		return ""
	}

	return hex.EncodeToString(uuid.Bytes[:])
}

func UUIDFromSiteKey(s string) pgtype.UUID {
	if len(s) != SitekeyLen {
		return invalidUUID
	}

	var result pgtype.UUID

	byteArray, err := hex.DecodeString(s)

	if (err == nil) && (len(byteArray) == len(result.Bytes)) {
		copy(result.Bytes[:], byteArray)
		result.Valid = true
		return result
	}

	return invalidUUID
}

func UUIDToSecret(uuid pgtype.UUID) string {
	if !uuid.Valid {
		return ""
	}

	return APIKeyPrefix + hex.EncodeToString(uuid.Bytes[:])
}

func UUIDFromSecret(s string) pgtype.UUID {
	if !strings.HasPrefix(s, APIKeyPrefix) {
		return invalidUUID
	}

	s = strings.TrimPrefix(s, APIKeyPrefix)

	if len(s) != SitekeyLen {
		return invalidUUID
	}

	var result pgtype.UUID

	byteArray, err := hex.DecodeString(s)

	if (err == nil) && (len(byteArray) == len(result.Bytes)) {
		copy(result.Bytes[:], byteArray)
		result.Valid = true
		return result
	}

	return invalidUUID
}

func fetchCachedOne[T any](ctx context.Context, cache common.Cache[CacheKey, any], key CacheKey) (*T, error) {
	data, err := cache.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	if t, ok := data.(*T); ok {
		return t, nil
	}

	return nil, errInvalidCacheType
}

func fetchCachedArray[T any](ctx context.Context, cache common.Cache[CacheKey, any], key CacheKey) ([]*T, error) {
	data, err := cache.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	if t, ok := data.([]*T); ok {
		return t, nil
	}

	return nil, errInvalidCacheType
}

func queryKeyInt(ck CacheKey) (int32, error) {
	return ck.IntValue, nil
}

func queryKeyStr(ck CacheKey) (string, error) {
	return ck.StrValue, nil
}

func queryKeySecretUUID(key CacheKey) (pgtype.UUID, error) {
	result := UUIDFromSecret(key.StrValue)
	if !result.Valid {
		return result, ErrInvalidInput
	}

	return result, nil
}

func queryKeySitekeyUUID(key CacheKey) (pgtype.UUID, error) {
	result := UUIDFromSiteKey(key.StrValue)
	if !result.Valid {
		return result, ErrInvalidInput
	}

	return result, nil
}

func queryKeyPgInt(key CacheKey) (pgtype.Int4, error) {
	return Int(key.IntValue), nil
}

type storeOneReader[TKey any, T any] struct {
	cacheKey     CacheKey
	queryFunc    func(context.Context, TKey) (*T, error)
	queryKeyFunc func(CacheKey) (TKey, error)
	cache        common.Cache[CacheKey, any]
}

func (sf *storeOneReader[TKey, T]) query(ctx context.Context, key CacheKey) (any, error) {
	if sf.queryFunc == nil {
		// in case of otter's refreshing, this should cause silent failure and eligibility for new refresh until item is expired
		// old item should be returned meanwhile
		return nil, ErrMaintenance
	}

	queryKey, err := sf.queryKeyFunc(key)
	if err != nil {
		return nil, err
	}

	t, err := sf.queryFunc(ctx, queryKey)
	if err != nil {
		if err == pgx.ErrNoRows {
			// this will cause cache to store this missing value and ultimately return ErrNegativeCacheHit
			// we do not return otter.ErrNotFound (as per docs), because in such case item will be purged from cache
			return sf.cache.Missing(), nil
		}

		slog.ErrorContext(ctx, "Failed to query value from DB", "cacheKey", key, common.ErrAttr(err))

		return nil, err
	}

	slog.Log(ctx, common.LevelTrace, "Retrieved entity from DB", "cacheKey", key)

	return t, nil
}

func (sf *storeOneReader[TKey, T]) Read(ctx context.Context) (*T, error) {
	data, err := sf.cache.GetEx(ctx, sf.cacheKey, sf.query)
	if err != nil {
		return nil, err
	}

	if t, ok := data.(*T); ok {
		return t, nil
	}

	return nil, errInvalidCacheType
}

type storeArrayReader[TKey any, T any] struct {
	key          CacheKey
	queryFunc    func(context.Context, TKey) ([]*T, error)
	queryKeyFunc func(CacheKey) (TKey, error)
	cache        common.Cache[CacheKey, any]
}

func (sf *storeArrayReader[TKey, T]) query(ctx context.Context, key CacheKey) (any, error) {
	if sf.queryFunc == nil {
		// in case of otter's refreshing, this should cause silent failure and eligibility for new refresh until item is expired
		// old item should be returned meanwhile
		return nil, ErrMaintenance
	}

	queryKey, err := sf.queryKeyFunc(key)
	if err != nil {
		return nil, err
	}

	t, err := sf.queryFunc(ctx, queryKey)
	if err != nil {
		if err == pgx.ErrNoRows {
			// unlike in case of one, we want to store empty array here and not "missing" value
			// because "no rows" is a valid result for "WHERE" query
			return []*T{}, nil
		}

		slog.ErrorContext(ctx, "Failed to query entities from DB", "cacheKey", key, common.ErrAttr(err))

		return nil, err
	}

	slog.Log(ctx, common.LevelTrace, "Retrieved entities from DB", "cacheKey", key, "count", len(t))

	return t, nil
}

func (sf *storeArrayReader[TKey, T]) Read(ctx context.Context) ([]*T, error) {
	data, err := sf.cache.GetEx(ctx, sf.key, sf.query)
	if err != nil {
		return nil, err
	}

	if t, ok := data.([]*T); ok {
		return t, nil
	}

	return nil, errInvalidCacheType
}
