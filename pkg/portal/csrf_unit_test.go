package portal

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/common"
	dbgen "github.com/PrivateCaptcha/PrivateCaptcha/pkg/db/generated"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/session"
)

type stubSessionStore struct {
	sessions map[string]*session.SessionData
	mu       sync.Mutex
}

func newStubSessionStore() *stubSessionStore {
	return &stubSessionStore{
		sessions: make(map[string]*session.SessionData),
	}
}

func (s *stubSessionStore) Start(context.Context, time.Duration) {}

func (s *stubSessionStore) Init(_ context.Context, sess *session.Session) error {
	s.mu.Lock()
	s.sessions[sess.ID()] = sess.Data()
	s.mu.Unlock()
	return nil
}

func (s *stubSessionStore) Read(_ context.Context, sid string, _ bool) (*session.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, ok := s.sessions[sid]
	if !ok {
		return nil, session.ErrSessionMissing
	}

	return session.NewSession(data, s), nil
}

func (s *stubSessionStore) Update(sess *session.Session) error {
	s.mu.Lock()
	s.sessions[sess.ID()] = sess.Data()
	s.mu.Unlock()
	return nil
}

func (s *stubSessionStore) Destroy(_ context.Context, sid string) error {
	s.mu.Lock()
	delete(s.sessions, sid)
	s.mu.Unlock()
	return nil
}

func TestCsrfHelpersUseSessionValues(t *testing.T) {
	store := newStubSessionStore()
	manager := &session.Manager{
		CookieName:  "pcsid",
		Store:       store,
		MaxLifetime: time.Minute,
	}
	manager.Init("portal", "/", time.Minute)

	s := &Server{
		XSRF: &common.XSRFMiddleware{
			Key:     "csrf-key",
			Timeout: time.Hour,
		},
		Sessions: manager,
	}

	user := &dbgen.User{ID: 7}
	csrfCtx := s.CreateCsrfContext(user)
	if csrfCtx.Token == "" {
		t.Fatalf("expected csrf token to be generated")
	}
	if !s.XSRF.VerifyToken(csrfCtx.Token, "7") {
		t.Fatalf("generated token is not valid for user")
	}

	req := httptest.NewRequest(http.MethodPost, "/submit", nil)
	w := httptest.NewRecorder()
	sess := manager.SessionStart(w, req)
	if err := sess.Set(session.KeyUserEmail, "user@example.com"); err != nil {
		t.Fatalf("failed to set session email: %v", err)
	}
	if err := sess.Set(session.KeyUserID, user.ID); err != nil {
		t.Fatalf("failed to set session user id: %v", err)
	}

	resp := w.Result()
	if len(resp.Cookies()) == 0 {
		t.Fatalf("session cookie not set")
	}
	cookie := resp.Cookies()[0]

	reqWithSession := httptest.NewRequest(http.MethodPost, "/submit", nil)
	reqWithSession.AddCookie(cookie)

	if email := s.csrfUserEmailKeyFunc(w, reqWithSession); email != "user@example.com" {
		t.Fatalf("expected email to be propagated from session, got %q", email)
	}

	userID := s.csrfUserIDKeyFunc(w, reqWithSession)
	if userID != "7" {
		t.Fatalf("expected user id 7, got %s", userID)
	}

	reqWithSession.Header.Set(common.HeaderCSRFToken, s.XSRF.Token(userID))
	nextCalled := false
	s.csrf(s.csrfUserIDKeyFunc)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		nextCalled = true
	})).ServeHTTP(httptest.NewRecorder(), reqWithSession)

	if !nextCalled {
		t.Fatalf("csrf middleware did not call next handler")
	}
}
