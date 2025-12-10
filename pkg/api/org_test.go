package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/common"
	common_test "github.com/PrivateCaptcha/PrivateCaptcha/pkg/common/tests"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/db"
	dbgen "github.com/PrivateCaptcha/PrivateCaptcha/pkg/db/generated"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/db/tests"
	db_test "github.com/PrivateCaptcha/PrivateCaptcha/pkg/db/tests"
	"github.com/rs/xid"
)

func requestResponseAPISuite[T any](request interface{}, method, endpoint, apiKey string) (T, *ResponseMetadata, error) {
	var zero T

	srv := http.NewServeMux()
	s.Setup("", true /*verbose*/, common.NoopMiddleware).Register(srv)

	//srv.HandleFunc("/", catchAll)

	var reader io.Reader
	if request != nil {
		data, err := json.Marshal(request)
		if err != nil {
			return zero, nil, err
		}
		reader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, endpoint, reader)
	if err != nil {
		return zero, nil, err
	}

	req.Header.Set(common.HeaderContentType, common.ContentTypeJSON)
	req.Header.Set(common.HeaderAPIKey, apiKey)
	req.Header.Set(cfg.Get(common.RateLimitHeaderKey).Value(), common_test.GenerateRandomIPv4())

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	resp := w.Result()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return zero, nil, err
	}

	var envelope APIResponse
	if err := json.Unmarshal(body, &envelope); err != nil {
		return zero, nil, err
	}

	raw, err := json.Marshal(envelope.Data)
	if err != nil {
		return zero, nil, err
	}

	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		return zero, nil, err
	}

	meta := new(ResponseMetadata)
	*meta = envelope.Meta

	return out, meta, nil
}

func setupAPISuite(username string) (*dbgen.User, *dbgen.Organization, string, error) {
	ctx := context.TODO()

	user, org, err := db_test.CreateNewAccountForTest(ctx, store, username, testPlan)
	if err != nil {
		return nil, nil, "", err
	}

	keyParams := tests.CreateNewPuzzleAPIKeyParams(username+"-apikey", time.Now(), 1*time.Hour, 10.0 /*rps*/)
	keyParams.Scope = dbgen.ApiKeyScopePortal
	apikey, _, err := store.Impl().CreateAPIKey(ctx, user, keyParams)
	if err != nil {
		return nil, nil, "", err
	}

	return user, org, db.UUIDToSecret(apikey.ExternalID), nil
}

func TestAPICreateOrg(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	user, baseOrg, apiKey, err := setupAPISuite(t.Name())
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.BusinessDB.Impl().SoftDeleteOrganization(context.TODO(), baseOrg, user); err != nil {
		t.Fatal(err)
	}

	input := &apiOrgInput{
		Name: t.Name(),
	}

	org, meta, err := requestResponseAPISuite[*apiOrgOutput](input, http.MethodPost, "/"+common.OrgEndpoint, apiKey)
	if err != nil {
		t.Fatal(err)
	}

	if !meta.Code.Success() {
		t.Fatalf("Unexpected status code: %v", meta.Description)
	}

	if len(org.ID) == 0 {
		t.Error("Created org ID is empty")
	}

	if org.Name != input.Name {
		t.Error("Created org name is different")
	}
}

func TestAPIDeleteOrg(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	user, org, apiKey, err := setupAPISuite(t.Name())
	if err != nil {
		t.Fatal(err)
	}

	input := &apiOrgInput{
		ID: s.IDHasher.Encrypt(int(org.ID)),
	}

	_, meta, err := requestResponseAPISuite[json.RawMessage](input, http.MethodDelete, "/"+common.OrgEndpoint, apiKey)
	if err != nil {
		t.Fatal(err)
	}

	if !meta.Code.Success() {
		t.Fatalf("Unexpected status code: %v", meta.Description)
	}

	if _, err := s.BusinessDB.Impl().RetrieveUserOrganization(context.TODO(), user, org.ID); (err != db.ErrSoftDeleted) && (err != db.ErrNegativeCacheHit) {
		t.Fatalf("Unexpected error when retrieving deleted org: %v", err)
	}
}

func TestAPIUpdateOrg(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	user, org, apiKey, err := setupAPISuite(t.Name())
	if err != nil {
		t.Fatal(err)
	}

	input := &apiOrgInput{
		ID:   s.IDHasher.Encrypt(int(org.ID)),
		Name: "Org Update " + xid.New().String(),
	}

	_, meta, err := requestResponseAPISuite[json.RawMessage](input, http.MethodPut, "/"+common.OrgEndpoint, apiKey)
	if err != nil {
		t.Fatal(err)
	}

	if !meta.Code.Success() {
		t.Fatalf("Unexpected status code: %v", meta.Description)
	}

	org, err = s.BusinessDB.Impl().RetrieveUserOrganization(context.TODO(), user, org.ID)
	if err != nil {
		t.Fatalf("Unexpected error when retrieving org: %v", err)
	}

	if org.Name != input.Name {
		t.Errorf("Org name was not updated. expected (%v), actual (%v)", input.Name, org.Name)
	}
}

func TestAPIGetOrgs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	_, org, apiKey, err := setupAPISuite(t.Name())
	if err != nil {
		t.Fatal(err)
	}

	orgs, meta, err := requestResponseAPISuite[[]*apiOrgOutput](nil, http.MethodGet, "/"+common.OrganizationsEndpoint, apiKey)
	if err != nil {
		t.Fatal(err)
	}

	if !meta.Code.Success() {
		t.Fatalf("Unexpected status code: %v", meta.Description)
	}

	if (len(orgs) != 1) || (orgs[0].Name != org.Name) {
		t.Errorf("Wrong organizations retrieved")
	}
}
