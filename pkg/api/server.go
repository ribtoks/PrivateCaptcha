package api

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/common"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/db"
	dbgen "github.com/PrivateCaptcha/PrivateCaptcha/pkg/db/generated"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/difficulty"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/monitoring"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/puzzle"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/ratelimit"
	"github.com/justinas/alice"
	"github.com/rs/cors"
)

const (
	maxSolutionsBodySize  = 256 * 1024
	VerifyBatchSize       = 100
	PropertyBucketSize    = 5 * time.Minute
	updateLimitsBatchSize = 100
	maxVerifyBatchSize    = 100_000
)

var (
	errAPIKeyNotSet  = errors.New("API key is not set in context")
	errInvalidAPIKey = errors.New("API key is not valid")
	errPuzzleOwner   = errors.New("error fetching puzzle owner")
	errInvalidArg    = errors.New("invalid arguments")
	errTestSolutions = errors.New("invalid test solutions")
	headersAnyOrigin = map[string][]string{
		http.CanonicalHeaderKey(common.HeaderAccessControlOrigin): []string{"*"},
		http.CanonicalHeaderKey(common.HeaderAccessControlAge):    []string{"86400"},
	}
	headersContentPlain = map[string][]string{
		http.CanonicalHeaderKey(common.HeaderContentType): []string{common.ContentTypePlain},
	}
)

type Server struct {
	Stage           string
	BusinessDB      db.Implementor
	TimeSeries      common.TimeSeriesStore
	Levels          *difficulty.Levels
	Auth            *AuthMiddleware
	VerifyLogChan   chan *common.VerifyRecord
	VerifyLogCancel context.CancelFunc
	Cors            *cors.Cors
	Metrics         common.APIMetrics
	Mailer          common.Mailer
	RateLimiter     ratelimit.HTTPRateLimiter
	Verifier        *Verifier
}

type apiKeyOwnerSource struct {
	Store     db.Implementor
	cachedKey *dbgen.APIKey
}

var _ puzzle.OwnerIDSource = (*apiKeyOwnerSource)(nil)

func (a *apiKeyOwnerSource) apiKey(ctx context.Context) (*dbgen.APIKey, error) {
	if apiKey, ok := ctx.Value(common.APIKeyContextKey).(*dbgen.APIKey); ok && (apiKey != nil) {
		a.cachedKey = apiKey
		return apiKey, nil
	}

	if secret, ok := ctx.Value(common.SecretContextKey).(string); ok && (len(secret) > 0) {
		// this is the "postponed" DB access mentioned in APIKey() middleware
		// NOTE: here we do NOT verify user's subscription validity, it's done only in middleware
		key, err := a.Store.Impl().RetrieveAPIKey(ctx, secret)
		if err != nil {
			a.cachedKey = key
		}
		return key, err
	}

	return nil, errAPIKeyNotSet
}

func (a *apiKeyOwnerSource) OwnerID(ctx context.Context, tnow time.Time) (int32, error) {
	apiKey, err := a.apiKey(ctx)
	if err != nil {
		return -1, err
	}

	if !isAPIKeyValid(ctx, apiKey, tnow) {
		return -1, errInvalidAPIKey
	}

	return apiKey.UserID.Int32, nil
}

type VerificationResponse struct {
	Success   bool               `json:"success"`
	Code      puzzle.VerifyError `json:"code"`
	Origin    string             `json:"origin,omitempty"`
	Timestamp common.JSONTime    `json:"timestamp,omitempty"`
}

type VerifyResponseRecaptchaV2 struct {
	Success     bool            `json:"success"`
	ErrorCodes  []string        `json:"error-codes,omitempty"`
	ChallengeTS common.JSONTime `json:"challenge_ts"`
	Hostname    string          `json:"hostname"`
}

type VerifyResponseRecaptchaV3 struct {
	VerifyResponseRecaptchaV2
	Score  float64 `json:"score"`
	Action string  `json:"action"`
}

func (s *Server) Init(ctx context.Context, verifyFlushInterval, authBackfillDelay time.Duration) error {
	if err := s.Verifier.Update(ctx); err != nil {
		slog.ErrorContext(ctx, "Failed to update puzzle verifier", common.ErrAttr(err))
		return err
	}

	s.Levels.Init(2*time.Second /*access log interval*/, PropertyBucketSize /*backfill interval*/)
	s.Auth.StartBackfill(authBackfillDelay)

	var cancelVerifyCtx context.Context
	cancelVerifyCtx, s.VerifyLogCancel = context.WithCancel(
		context.WithValue(context.Background(), common.TraceIDContextKey, "flush_verify_log"))

	go common.ProcessBatchArray(cancelVerifyCtx, s.VerifyLogChan, verifyFlushInterval, VerifyBatchSize, maxVerifyBatchSize, s.TimeSeries.WriteVerifyLogBatch)

	return nil
}

func (s *Server) Setup(router *http.ServeMux, domain string, verbose bool, security alice.Constructor) {
	corsOpts := cors.Options{
		// NOTE: due to the implementation of rs/cors, we need not to set "*" as AllowOrigin as this will ruin the response
		// (in case of "*" allowed origin, response contains the same, while we want to restrict the response to domain)
		AllowOriginVaryRequestFunc: s.Auth.originAllowed,
		AllowedHeaders:             []string{common.HeaderCaptchaVersion, "accept", "content-type", "x-requested-with"},
		AllowedMethods:             []string{http.MethodGet},
		AllowPrivateNetwork:        true,
		OptionsPassthrough:         true,
		Debug:                      verbose,
		MaxAge:                     60 * 60, /*seconds*/
	}

	if corsOpts.Debug {
		corsOpts.Logger = &common.FmtLogger{Ctx: common.TraceContext(context.TODO(), "cors"), Level: common.LevelTrace}
	}

	s.Cors = cors.New(corsOpts)

	s.setupWithPrefix(domain, router, s.Cors.Handler, security)
}

func (s *Server) Shutdown() {
	s.Levels.Shutdown()
	s.Auth.Shutdown()

	slog.Debug("Shutting down API server routines")
	s.VerifyLogCancel()
	close(s.VerifyLogChan)
}

func (s *Server) setupWithPrefix(domain string, router *http.ServeMux, corsHandler, security alice.Constructor) {
	prefix := domain + "/"
	slog.Debug("Setting up the API routes", "prefix", prefix)
	publicChain := alice.New(common.Recovered, security, s.Metrics.Handler)
	// NOTE: auth middleware provides rate limiting internally
	puzzleChain := publicChain.Append(s.RateLimiter.RateLimit, monitoring.Traced, common.TimeoutHandler(1*time.Second))
	router.Handle(http.MethodGet+" "+prefix+common.PuzzleEndpoint, puzzleChain.Append(corsHandler, s.Auth.Sitekey).ThenFunc(s.puzzleHandler))
	router.Handle(http.MethodOptions+" "+prefix+common.PuzzleEndpoint, puzzleChain.Append(common.Cached, corsHandler, s.Auth.SitekeyOptions).ThenFunc(s.puzzlePreFlight))

	const (
		// NOTE: these defaults will be adjusted per API key quota almost immediately after verifying API key
		// requests burst
		apiKeyLeakyBucketCap = 10
		// effective 0.5 rps
		apiKeyLeakInterval = 2 * time.Second
	)
	apiRateLimiter := s.RateLimiter.RateLimitExFunc(apiKeyLeakyBucketCap, apiKeyLeakInterval)

	verifyChain := publicChain.Append(apiRateLimiter, monitoring.Traced, common.TimeoutHandler(5*time.Second))
	// reCAPTCHA compatibility
	// the difference from our side is _when_ we fetch API key: for reCAPTCHA it comes in form field "secret" and
	// we want to put it _behind_ the MaxBytesHandler, while for Private Captcha format (header) it can be before
	formAPIAuth := s.Auth.APIKey(formSecretAPIKey)
	router.Handle(http.MethodPost+" "+prefix+common.SiteVerifyEndpoint, verifyChain.Then(http.MaxBytesHandler(formAPIAuth(http.HandlerFunc(s.recaptchaVerifyHandler)), maxSolutionsBodySize)))
	// Private Captcha format
	router.Handle(http.MethodPost+" "+prefix+common.VerifyEndpoint, verifyChain.Append(s.Auth.APIKey(headerAPIKey)).Then(http.MaxBytesHandler(http.HandlerFunc(s.pcVerifyHandler), maxSolutionsBodySize)))

	// "root" access
	router.Handle(prefix+"{$}", publicChain.Then(common.HttpStatus(http.StatusForbidden)))
}

func (s *Server) puzzlePreFlight(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// the reason for this is that we intend to cache test property responses
	if sitekey, ok := ctx.Value(common.SitekeyContextKey).(string); ok && (sitekey == db.TestPropertySitekey) {
		common.WriteHeaders(w, headersAnyOrigin)
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) puzzleHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	puzzle, property, err := s.Verifier.PuzzleForRequest(r, s.Levels)
	if err != nil {
		if err == db.ErrTestProperty {
			common.WriteHeaders(w, common.CachedHeaders)
			// we cache test property responses, can as well allow them anywhere
			common.WriteHeaders(w, headersAnyOrigin)
			common.WriteHeaders(w, headersContentPlain)
			_ = s.Verifier.WriteTestPuzzle(w)
			return
		}

		status := http.StatusInternalServerError
		if err == errInvalidArg {
			status = http.StatusBadRequest
		} else {
			slog.ErrorContext(ctx, "Failed to create puzzle", common.ErrAttr(err))
		}

		http.Error(w, "", status)
		return
	}

	var extraSalt []byte
	var userID int32 = -1
	if property != nil {
		userID = property.OrgOwnerID.Int32
		extraSalt = property.Salt
	}

	if err := s.Verifier.Write(ctx, puzzle, extraSalt, w); err != nil {
		slog.ErrorContext(ctx, "Failed to write puzzle", common.ErrAttr(err))
	}

	s.Metrics.ObservePuzzleCreated(userID)
}

// reCAPTCHA format: puzzle response is in form field "response", API key is in form field "secret"
// https://developers.google.com/recaptcha/docs/verify
func (s *Server) recaptchaVerifyHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	data := r.FormValue(common.ParamResponse)
	if len(data) == 0 {
		slog.ErrorContext(ctx, "Empty captcha response")
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	payload, err := s.Verifier.ParseSolutionPayload(ctx, []byte(data))
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	ownerSource := &apiKeyOwnerSource{Store: s.BusinessDB}
	result, err := s.Verifier.Verify(ctx, payload, ownerSource, time.Now().UTC())
	if err != nil {
		switch err {
		case errPuzzleOwner:
			// "late" auth check (we postpone API key check in case it's not cached in Auth)
			// in this case we also automatically set "API key" (or whatever is passed) as missing in cache
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		default:
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		}
		return
	}

	if result.Valid() {
		s.addVerifyRecord(ctx, result)
	}

	if apiKey := ownerSource.cachedKey; apiKey != nil {
		// if we are not cached, then we will recheck via "delayed" mechanism of OwnerIDSource
		// when rate limiting is cleaned up (due to inactivity) we should still be able to access on defaults
		interval := float64(time.Second) / apiKey.RequestsPerSecond
		s.RateLimiter.UpdateRequestLimits(r, uint32(apiKey.RequestsBurst), time.Duration(interval))
	}

	vr2 := &VerifyResponseRecaptchaV2{
		Success:     result.Success(),
		ErrorCodes:  result.ErrorsToStrings(),
		ChallengeTS: common.JSONTime(result.CreatedAt),
		Hostname:    result.Domain,
	}

	var response interface{} = vr2
	if recaptchaCompatVersion := r.Header.Get(common.HeaderCaptchaCompat); recaptchaCompatVersion == "rcV3" {
		response = &VerifyResponseRecaptchaV3{
			VerifyResponseRecaptchaV2: *vr2,
			Action:                    "",
			Score:                     0.5,
		}
	}

	common.SendJSONResponse(r.Context(), w, response, common.NoCacheHeaders)
}

// Private Captcha format: puzzle response is the whole body, API key is in header
func (s *Server) pcVerifyHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	data, err := io.ReadAll(r.Body)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to read request body", common.ErrAttr(err))
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	payload, err := s.Verifier.ParseSolutionPayload(ctx, data)
	if err != nil {
		slog.Log(ctx, common.LevelTrace, "Failed to parse solution payload", common.ErrAttr(err))
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	ownerSource := &apiKeyOwnerSource{Store: s.BusinessDB}
	result, err := s.Verifier.Verify(ctx, payload, ownerSource, time.Now().UTC())
	if err != nil {
		switch err {
		case errPuzzleOwner:
			// "late" auth check (we postpone API key check in case it's not cached in Auth)
			// in this case we also automatically set "API key" (or whatever is passed) as missing in cache
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		default:
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		}
		return
	}

	if result.Valid() {
		s.addVerifyRecord(ctx, result)
	}

	if apiKey := ownerSource.cachedKey; apiKey != nil {
		// if we are not cached, then we will recheck via "delayed" mechanism of OwnerIDSource
		// when rate limiting is cleaned up (due to inactivity) we should still be able to access on defaults
		interval := float64(time.Second) / apiKey.RequestsPerSecond
		s.RateLimiter.UpdateRequestLimits(r, uint32(apiKey.RequestsBurst), time.Duration(interval))
	}

	response := &VerificationResponse{
		Success:   result.Success(),
		Code:      result.Error,
		Origin:    result.Domain,
		Timestamp: common.JSONTime(result.CreatedAt),
	}

	common.SendJSONResponse(r.Context(), w, response, common.NoCacheHeaders)
}

func (s *Server) addVerifyRecord(ctx context.Context, result *puzzle.VerifyResult) {
	vr := &common.VerifyRecord{
		UserID:     result.UserID,
		OrgID:      result.OrgID,
		PropertyID: result.PropertyID,
		PuzzleID:   result.PuzzleID,
		Timestamp:  time.Now().UTC(),
		Status:     int8(result.Error),
	}

	s.VerifyLogChan <- vr

	s.Metrics.ObservePuzzleVerified(vr.UserID, result.Error.String(), (result.PuzzleID == 0) /*is stub*/)
}
