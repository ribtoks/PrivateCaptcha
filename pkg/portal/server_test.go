package portal

import (
	"context"
	"database/sql"
	"flag"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/billing"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/common"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/config"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/db"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/difficulty"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/email"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/maintenance"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/monitoring"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/puzzle"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/ratelimit"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/session"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/session/store/memory"
	"github.com/PrivateCaptcha/PrivateCaptcha/web"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	server     *Server
	cfg        common.ConfigStore
	timeSeries *db.TimeSeriesDB
	store      *db.BusinessStore
	testPlan   billing.Plan
	cache      common.Cache[db.CacheKey, any]
)

func portalDomain() string {
	return config.AsURL(context.TODO(), cfg.Get(common.PortalBaseURLKey)).Domain()
}

type fakePuzzleEngine struct {
	result *puzzle.VerifyResult
}

func (f *fakePuzzleEngine) ParseSolutionPayload(ctx context.Context, data []byte) (puzzle.SolutionPayload, error) {
	return puzzle.NewStubPayload(puzzle.NewComputePuzzle(0, [puzzle.PropertyIDSize]byte{}, 0)), nil
}

func (f *fakePuzzleEngine) Create(puzzleID uint64, propertyID [puzzle.PropertyIDSize]byte, difficulty uint8) puzzle.Puzzle {
	return puzzle.NewComputePuzzle(puzzleID, propertyID, difficulty)
}

func (f *fakePuzzleEngine) Write(ctx context.Context, p puzzle.Puzzle, extraSalt []byte, w http.ResponseWriter) error {
	return nil
}

func (f *fakePuzzleEngine) Verify(ctx context.Context, payload puzzle.SolutionPayload, expectedOwner puzzle.OwnerIDSource, tnow time.Time) (*puzzle.VerifyResult, error) {
	return f.result, nil
}

func TestMain(m *testing.M) {
	flag.Parse()

	planService := billing.NewPlanService(nil)
	testPlan = planService.GetInternalTrialPlan()

	dataCtx, err := web.LoadData()
	if err != nil {
		panic(err)
	}

	if testing.Short() {
		server = &Server{
			Stage:  common.StageTest,
			Prefix: "",
			XSRF:   &common.XSRFMiddleware{Key: "key", Timeout: 1 * time.Hour},
			Sessions: &session.Manager{
				CookieName:  "pcsid",
				MaxLifetime: 1 * time.Minute,
			},
			PuzzleEngine: &fakePuzzleEngine{result: &puzzle.VerifyResult{Error: puzzle.VerifyNoError}},
			PlanService:  planService,
			DataCtx:      dataCtx,
		}

		ctx := context.TODO()
		templatesBuilder := NewTemplatesBuilder()
		templatesBuilder.AddFS(ctx, web.Templates(), "core")

		if err := server.Init(ctx, templatesBuilder); err != nil {
			panic(err)
		}

		os.Exit(m.Run())
	}

	common.SetupLogs(common.StageTest, true)

	cfg = config.NewEnvConfig(os.Getenv)

	var pool *pgxpool.Pool
	var clickhouse *sql.DB
	var dberr error
	pool, clickhouse, dberr = db.Connect(context.Background(), cfg, 3*time.Second, false /*admin*/)
	if dberr != nil {
		panic(dberr)
	}

	timeSeries = db.NewTimeSeries(clickhouse)

	levels := difficulty.NewLevels(timeSeries, 100, 5*time.Minute)
	levels.Init(2*time.Second, 5*time.Minute)
	defer levels.Shutdown()

	cache, err = db.NewMemoryCache[db.CacheKey, any]("default", 100, &struct{}{}, 1*time.Minute, 3*time.Minute, 30*time.Second)
	if err != nil {
		panic(err)
	}

	store = db.NewBusinessEx(pool, cache)

	sessionStore := db.NewSessionStore(pool, memory.New(), 1*time.Minute, session.KeyPersistent)

	server = &Server{
		Stage:      common.StageTest,
		Store:      store,
		TimeSeries: timeSeries,
		Prefix:     "",
		XSRF:       &common.XSRFMiddleware{Key: "key", Timeout: 1 * time.Hour},
		Sessions: &session.Manager{
			CookieName:  "pcsid",
			Store:       sessionStore,
			MaxLifetime: sessionStore.MaxLifetime(),
		},
		Mailer:       &email.StubMailer{},
		RateLimiter:  &ratelimit.StubRateLimiter{Header: cfg.Get(common.RateLimitHeaderKey).Value()},
		PuzzleEngine: &fakePuzzleEngine{result: &puzzle.VerifyResult{Error: puzzle.VerifyNoError}},
		Metrics:      monitoring.NewStub(),
		PlanService:  planService,
		DataCtx:      dataCtx,
	}

	ctx := context.TODO()
	templatesBuilder := NewTemplatesBuilder()
	if err := templatesBuilder.AddFS(ctx, web.Templates(), "core"); err != nil {
		panic(err)
	}

	if err := server.Init(ctx, templatesBuilder); err != nil {
		panic(err)
	}

	job := &maintenance.RegisterEmailTemplatesJob{
		Templates: email.Templates(),
		Store:     store,
	}
	job.RunOnce(ctx, job.NewParams())

	os.Exit(m.Run())
}
