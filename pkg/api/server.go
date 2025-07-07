package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/netip"
	"time"

	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/common"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/db"
	dbgen "github.com/PrivateCaptcha/PrivateCaptcha/pkg/db/generated"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/difficulty"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/monitoring"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/puzzle"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/justinas/alice"
	"github.com/rs/cors"
	"golang.org/x/crypto/blake2b"
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
	headersAnyOrigin = map[string][]string{
		http.CanonicalHeaderKey(common.HeaderAccessControlOrigin): []string{"*"},
		http.CanonicalHeaderKey(common.HeaderAccessControlAge):    []string{"86400"},
	}
	headersContentPlain = map[string][]string{
		http.CanonicalHeaderKey(common.HeaderContentType): []string{common.ContentTypePlain},
	}
	verifyResultErrorTest = &puzzle.VerifyResult{
		Errors: []puzzle.VerifyError{puzzle.TestPropertyError},
	}
	verifyResultErrorParse = &puzzle.VerifyResult{
		Errors: []puzzle.VerifyError{puzzle.ParseResponseError},
	}
)

type Server struct {
	Stage              string
	BusinessDB         db.Implementor
	TimeSeries         common.TimeSeriesStore
	Levels             *difficulty.Levels
	Auth               *AuthMiddleware
	UserFingerprintKey *userFingerprintKey
	Salt               *puzzleSalt
	VerifyLogChan      chan *common.VerifyRecord
	VerifyLogCancel    context.CancelFunc
	Cors               *cors.Cors
	Metrics            common.APIMetrics
	Mailer             common.Mailer
	TestPuzzle         *puzzle.Puzzle
	TestPuzzleData     *puzzle.PuzzlePayload
}

var _ puzzle.Engine = (*Server)(nil)

type apiKeyOwnerSource struct {
	Store db.Implementor
}

var _ puzzle.OwnerIDSource = (*apiKeyOwnerSource)(nil)

func (a *apiKeyOwnerSource) apiKey(ctx context.Context) (*dbgen.APIKey, error) {
	if apiKey, ok := ctx.Value(common.APIKeyContextKey).(*dbgen.APIKey); ok {
		return apiKey, nil
	}

	if secret, ok := ctx.Value(common.SecretContextKey).(string); ok {
		// this is the "postponed" DB access mentioned in APIKey() middleware
		return a.Store.Impl().RetrieveAPIKey(ctx, secret)
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

type VerifyResponse struct {
	Success    bool     `json:"success"`
	ErrorCodes []string `json:"error-codes,omitempty"`
}

type VerifyResponseRecaptchaV2 struct {
	VerifyResponse
	ChallengeTS common.JSONTime `json:"challenge_ts"`
	Hostname    string          `json:"hostname"`
}

type VerifyResponseRecaptchaV3 struct {
	VerifyResponseRecaptchaV2
	Score  float64 `json:"score"`
	Action string  `json:"action"`
}

func (s *Server) Init(ctx context.Context, verifyFlushInterval, authBackfillDelay time.Duration) error {
	if err := s.Salt.Update(); err != nil {
		slog.ErrorContext(ctx, "Failed to update puzzle salt", common.ErrAttr(err))
		return err
	}

	if err := s.UserFingerprintKey.Update(); err != nil {
		slog.ErrorContext(ctx, "Failed to update user fingerprint key", common.ErrAttr(err))
		return err
	}

	s.TestPuzzle = puzzle.NewPuzzle(0 /*puzzle ID*/, db.TestPropertyUUID.Bytes, 0 /*difficulty*/)
	var err error
	s.TestPuzzleData, err = s.TestPuzzle.Serialize(ctx, s.Salt.Value(), nil /*property salt*/)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to serialize test puzzle", common.ErrAttr(err))
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
	publicChain := alice.New(common.Recovered, monitoring.Traced, security, s.Metrics.Handler)
	// NOTE: auth middleware provides rate limiting internally
	router.Handle(http.MethodGet+" "+prefix+common.PuzzleEndpoint, publicChain.Append(corsHandler, common.TimeoutHandler(1*time.Second), s.Auth.Sitekey).ThenFunc(s.puzzleHandler))
	router.Handle(http.MethodOptions+" "+prefix+common.PuzzleEndpoint, publicChain.Append(common.Cached, corsHandler, s.Auth.SitekeyOptions).ThenFunc(s.puzzlePreFlight))
	verifyChain := publicChain.Append(common.TimeoutHandler(5*time.Second), s.Auth.APIKey)
	router.Handle(http.MethodPost+" "+prefix+common.VerifyEndpoint, verifyChain.Then(http.MaxBytesHandler(http.HandlerFunc(s.verifyHandler), maxSolutionsBodySize)))

	// "root" access
	router.Handle(prefix+"{$}", publicChain.Then(common.HttpStatus(http.StatusForbidden)))
}

func (s *Server) puzzleForRequest(r *http.Request) (*puzzle.Puzzle, *dbgen.Property, error) {
	ctx := r.Context()
	property, ok := ctx.Value(common.PropertyContextKey).(*dbgen.Property)
	// property will not be cached for auth.backfillDelay and we return an "average" puzzle instead
	// this is done in order to not check the DB on the hot path (decrease attack surface)
	if !ok {
		sitekey := ctx.Value(common.SitekeyContextKey).(string)
		if sitekey == db.TestPropertySitekey {
			return nil, nil, db.ErrTestProperty
		}

		uuid := db.UUIDFromSiteKey(sitekey)
		// if it's a legit request, then puzzle will be also legit (verifiable) with this PropertyID
		stubPuzzle := puzzle.NewPuzzle(0 /*puzzle ID*/, uuid.Bytes, uint8(common.DifficultyLevelMedium))
		if err := stubPuzzle.Init(puzzle.DefaultValidityPeriod); err != nil {
			slog.ErrorContext(ctx, "Failed to init stub puzzle", common.ErrAttr(err))
		}

		slog.Log(ctx, common.LevelTrace, "Returning stub puzzle before auth is backfilled", "puzzleID", stubPuzzle.PuzzleID,
			"sitekey", sitekey, "difficulty", stubPuzzle.Difficulty)
		return stubPuzzle, nil, nil
	}

	var fingerprint common.TFingerprint
	hash, err := blake2b.New256(s.UserFingerprintKey.Value())
	if err != nil {
		slog.ErrorContext(ctx, "Failed to create blake2b hmac", common.ErrAttr(err))
		fingerprint = common.RandomFingerprint()
	} else {
		// TODO: Check if we really need to take user agent into account here
		// or it should be accounted on the anomaly detection side (user-agent is trivial to spoof)
		// hash.Write([]byte(r.UserAgent()))
		if ip, ok := ctx.Value(common.RateLimitKeyContextKey).(netip.Addr); ok && ip.IsValid() {
			hash.Write(ip.AsSlice())
		} else {
			slog.ErrorContext(ctx, "Rate limit context key type mismatch", "ip", ip)
			hash.Write([]byte(r.RemoteAddr))
		}
		hmac := hash.Sum(nil)
		truncatedHmac := hmac[:8]
		fingerprint = binary.BigEndian.Uint64(truncatedHmac)
	}

	tnow := time.Now()
	puzzleDifficulty := s.Levels.Difficulty(fingerprint, property, tnow)

	puzzleID := puzzle.RandomPuzzleID()
	result := puzzle.NewPuzzle(puzzleID, property.ExternalID.Bytes, puzzleDifficulty)
	if err := result.Init(property.ValidityInterval); err != nil {
		slog.ErrorContext(ctx, "Failed to init puzzle", common.ErrAttr(err))
	}

	slog.Log(ctx, common.LevelTrace, "Prepared new puzzle", "propertyID", property.ID, "difficulty", result.Difficulty,
		"puzzleID", result.PuzzleID, "userID", property.OrgOwnerID.Int32)

	return result, property, nil
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
	puzzle, property, err := s.puzzleForRequest(r)
	if err != nil {
		if err == db.ErrTestProperty {
			common.WriteHeaders(w, common.CachedHeaders)
			// we cache test property responses, can as well allow them anywhere
			common.WriteHeaders(w, headersAnyOrigin)
			common.WriteHeaders(w, headersContentPlain)
			_ = s.TestPuzzleData.Write(w)
			return
		}

		slog.ErrorContext(ctx, "Failed to create puzzle", common.ErrAttr(err))
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	var extraSalt []byte
	var userID int32 = -1
	if property != nil {
		userID = property.OrgOwnerID.Int32
		extraSalt = property.Salt
	}

	if err := s.Write(ctx, puzzle, extraSalt, w); err != nil {
		slog.ErrorContext(ctx, "Failed to write puzzle", common.ErrAttr(err))
	}

	s.Metrics.ObservePuzzleCreated(userID)
}

func (s *Server) Write(ctx context.Context, p *puzzle.Puzzle, extraSalt []byte, w http.ResponseWriter) error {
	payload, err := p.Serialize(ctx, s.Salt.Value(), extraSalt)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return err
	}

	common.WriteHeaders(w, common.NoCacheHeaders)
	common.WriteHeaders(w, headersContentPlain)
	return payload.Write(w)
}

func (s *Server) Verify(ctx context.Context, data []byte, expectedOwner puzzle.OwnerIDSource, tnow time.Time) (*puzzle.VerifyResult, error) {
	// this is faster than doing base64 decoding and parsing of zero puzzle
	if s.TestPuzzleData.IsPrefixFor(data) {
		// lazy roughly check solutions
		solutionsBase64Size := len(data) - s.TestPuzzleData.Size()
		slog.Log(ctx, common.LevelTrace, "Detected test puzzle prefix in verify payload", "remaining", solutionsBase64Size)
		solutionsMaxSize := base64.StdEncoding.DecodedLen(solutionsBase64Size)
		if solutionsMaxSize < int(s.TestPuzzle.SolutionsCount)*puzzle.SolutionLength {
			return verifyResultErrorParse, nil
		}
		return verifyResultErrorTest, nil
	}

	verifyPayload, err := puzzle.ParseVerifyPayload(ctx, string(data))
	if err != nil {
		slog.WarnContext(ctx, "Failed to parse verify payload", common.ErrAttr(err))
		return verifyResultErrorParse, nil
	}

	result := &puzzle.VerifyResult{Errors: make([]puzzle.VerifyError, 0, 1)}
	puzzleObject, property, perr := s.verifyPuzzleValid(ctx, verifyPayload, tnow)
	result.AddError(perr)
	if puzzleObject != nil && !puzzleObject.IsZero() {
		result.CreatedAt = puzzleObject.Expiration.Add(-puzzle.DefaultValidityPeriod)
	}
	if property != nil {
		result.Domain = property.Domain
	}
	if perr != puzzle.VerifyNoError && perr != puzzle.MaintenanceModeError {
		return result, nil
	}

	if property != nil {
		// position in code where expected owner is checked is a tradeoff between compute for verifying solutions (below)
		// and IO for accessing DB of potentially malicious request (in case not-yet-checked API key turns out invalid)
		if ownerID, err := expectedOwner.OwnerID(ctx, tnow); err == nil {
			if (property.OrgOwnerID.Int32 != ownerID) && (property.CreatorID.Int32 != ownerID) {
				slog.WarnContext(ctx, "Org owner does not match expected owner", "expectedOwner", ownerID,
					"orgOwner", property.OrgOwnerID.Int32, "propertyCreator", property.CreatorID.Int32)
				result.AddError(puzzle.WrongOwnerError)
				return result, nil
			}
		} else {
			slog.ErrorContext(ctx, "Failed to fetch owner ID", "puzzleID", puzzleObject.PuzzleID, common.ErrAttr(err))
			return nil, errPuzzleOwner
		}
	}

	if metadata, verr := verifyPayload.VerifySolutions(ctx); verr != puzzle.VerifyNoError {
		// NOTE: unlike solutions/puzzle, diagnostics bytes can be totally tampered
		slog.WarnContext(ctx, "Failed to verify solutions", "result", verr.String(), "clientError", metadata.ErrorCode(),
			"elapsedMillis", metadata.ElapsedMillis(), "puzzleID", puzzleObject.PuzzleID, "userID", property.OrgOwnerID.Int32,
			"propertyID", property.ID)

		s.addVerifyRecord(ctx, puzzleObject, property, verr)
		result.AddError(verr)
		return result, nil
	}

	if (puzzleObject != nil) && (property != nil) && !property.AllowReplay {
		if cerr := s.BusinessDB.CachePuzzle(ctx, puzzleObject, tnow); cerr != nil {
			slog.ErrorContext(ctx, "Failed to cache puzzle", "puzzleID", puzzleObject.PuzzleID, common.ErrAttr(cerr))
		}
	}

	s.addVerifyRecord(ctx, puzzleObject, property, puzzle.VerifyNoError)

	return result, nil
}

func (s *Server) verifyHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	data, err := io.ReadAll(r.Body)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to read request body", common.ErrAttr(err))
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	result, err := s.Verify(ctx, data, &apiKeyOwnerSource{Store: s.BusinessDB}, time.Now().UTC())
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

	vr2 := &VerifyResponseRecaptchaV2{
		VerifyResponse: VerifyResponse{
			Success:    result.Success(),
			ErrorCodes: result.ErrorsToStrings(),
		},
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

	common.SendJSONResponse(ctx, w, response, common.NoCacheHeaders)
}

func (s *Server) addVerifyRecord(ctx context.Context, p *puzzle.Puzzle, property *dbgen.Property, verr puzzle.VerifyError) {
	if (p == nil) || (property == nil) {
		slog.WarnContext(ctx, "Invalid input for verify record", "property", (property != nil), "puzzle", (p != nil))
		return
	}

	vr := &common.VerifyRecord{
		UserID:     property.OrgOwnerID.Int32,
		OrgID:      property.OrgID.Int32,
		PropertyID: property.ID,
		PuzzleID:   p.PuzzleID,
		Timestamp:  time.Now().UTC(),
		Status:     int8(verr),
	}

	s.VerifyLogChan <- vr

	s.Metrics.ObservePuzzleVerified(vr.UserID, verr.String(), p.IsStub())
}

func (s *Server) verifyPuzzleValid(ctx context.Context, payload *puzzle.VerifyPayload, tnow time.Time) (*puzzle.Puzzle, *dbgen.Property, puzzle.VerifyError) {
	p := payload.Puzzle()
	plog := slog.With("puzzleID", p.PuzzleID)

	if p.IsZero() && bytes.Equal(p.PropertyID[:], db.TestPropertyUUID.Bytes[:]) {
		plog.DebugContext(ctx, "Verifying test puzzle")
		return p, nil, puzzle.TestPropertyError
	}

	if !tnow.Before(p.Expiration) {
		plog.WarnContext(ctx, "Puzzle is expired", "expiration", p.Expiration, "now", tnow)
		return p, nil, puzzle.PuzzleExpiredError
	}

	// "else" branch is handled below _after_ we fetch the property from DB
	if !payload.NeedsExtraSalt() {
		if serr := payload.VerifySignature(ctx, s.Salt.Value(), nil /*extra salt*/); serr != nil {
			return p, nil, puzzle.IntegrityError
		}
	}

	if s.BusinessDB.CheckPuzzleCached(ctx, p) {
		plog.WarnContext(ctx, "Puzzle is already cached")
		return p, nil, puzzle.VerifiedBeforeError
	}

	// the reason we delay accessing DB for API key and not for sitekey is that sitekey comes from a signed puzzle payload
	// and API key is a rather random string in HTTP header so has a higher chance of misuse
	sitekey := db.UUIDToSiteKey(pgtype.UUID{Valid: true, Bytes: p.PropertyID})
	property, err := s.BusinessDB.Impl().RetrievePropertyBySitekey(ctx, sitekey)
	if err != nil {
		switch err {
		case db.ErrNegativeCacheHit, db.ErrRecordNotFound, db.ErrSoftDeleted:
			return p, nil, puzzle.InvalidPropertyError
		case db.ErrMaintenance:
			return p, nil, puzzle.MaintenanceModeError
		default:
			plog.ErrorContext(ctx, "Failed to find property by sitekey", "sitekey", sitekey, common.ErrAttr(err))
			return p, nil, puzzle.VerifyErrorOther
		}
	}

	if payload.NeedsExtraSalt() {
		if serr := payload.VerifySignature(ctx, s.Salt.Value(), property.Salt); serr != nil {
			return p, nil, puzzle.IntegrityError
		}
	}

	return p, property, puzzle.VerifyNoError
}
