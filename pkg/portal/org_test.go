package portal

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/common"
	dbgen "github.com/PrivateCaptcha/PrivateCaptcha/pkg/db/generated"
	db_tests "github.com/PrivateCaptcha/PrivateCaptcha/pkg/db/tests"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/email"
	portal_tests "github.com/PrivateCaptcha/PrivateCaptcha/pkg/portal/tests"
)

func TestGetAnotherUsersOrg(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := context.TODO()
	_, org1, err := db_tests.CreateNewAccountForTest(ctx, store, t.Name()+"_1", testPlan)
	if err != nil {
		t.Fatalf("Failed to create owner account: %v", err)
	}

	// Create another user account
	user2, _, err := db_tests.CreateNewAccountForTest(ctx, store, t.Name()+"_2", testPlan)
	if err != nil {
		t.Fatalf("Failed to create intruder account: %v", err)
	}

	srv := http.NewServeMux()
	_ = server.Setup(srv, portalDomain(), common.NoopMiddleware)

	cookie, err := portal_tests.AuthenticateSuite(ctx, user2.Email, srv, server.XSRF, server.Sessions.CookieName, server.Mailer.(*email.StubMailer))
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", fmt.Sprintf("/org/%d/%s/%s", org1.ID, common.TabEndpoint, common.DashboardEndpoint), nil)
	req.AddCookie(cookie)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Errorf("Unexpected status code %v", resp.StatusCode)
	}

	url, _ := resp.Location()
	if path := url.String(); !strings.HasPrefix(path, "/"+common.ErrorEndpoint) {
		t.Errorf("Unexpected redirect: %s", path)
	}
}

func TestInviteUser(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := context.TODO()
	user1, org1, err := db_tests.CreateNewAccountForTest(ctx, store, t.Name()+"_1", testPlan)
	if err != nil {
		t.Fatalf("Failed to create owner account: %v", err)
	}

	// Create another user account
	user2, _, err := db_tests.CreateNewAccountForTest(ctx, store, t.Name()+"_2", testPlan)
	if err != nil {
		t.Fatalf("Failed to create invitee account: %v", err)
	}

	srv := http.NewServeMux()
	_ = server.Setup(srv, portalDomain(), common.NoopMiddleware)

	cookie, err := portal_tests.AuthenticateSuite(ctx, user1.Email, srv, server.XSRF, server.Sessions.CookieName, server.Mailer.(*email.StubMailer))
	if err != nil {
		t.Fatal(err)
	}

	form := url.Values{}
	form.Set(common.ParamCSRFToken, server.XSRF.Token(strconv.Itoa(int(user1.ID))))
	form.Set(common.ParamEmail, user2.Email)

	req := httptest.NewRequest("POST", fmt.Sprintf("/org/%d/members", org1.ID), strings.NewReader(form.Encode()))
	req.AddCookie(cookie)
	req.Header.Set(common.HeaderContentType, common.ContentTypeURLEncoded)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Unexpected status code %v", resp.StatusCode)
	}

	members, err := store.Impl().RetrieveOrganizationUsers(ctx, org1.ID)
	if err != nil {
		t.Fatal(err)
	}

	if len(members) != 1 {
		t.Errorf("Unexpected length of members: %v", len(members))
	}

	member := members[0]
	if (member.User.ID != user2.ID) && (member.Level != dbgen.AccessLevelInvited) {
		t.Errorf("Org member is not invited user")
	}
}
