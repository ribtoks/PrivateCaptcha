//go:build enterprise

package api

import (
	"context"
	"net/http"
	"time"

	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/common"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/db"
	dbgen "github.com/PrivateCaptcha/PrivateCaptcha/pkg/db/generated"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/monitoring"
	"github.com/justinas/alice"
)

func (s *Server) setupEnterprise(rg *common.RouteGenerator, publicChain alice.Chain, apiRateLimiter func(next http.Handler) http.Handler) {
	// "portal" API
	portalAPIChain := publicChain.Append(apiRateLimiter, monitoring.Traced, common.TimeoutHandler(5*time.Second), s.Auth.APIKey(headerAPIKey, dbgen.ApiKeyScopePortal))
	rg.Handle(rg.Get(common.OrganizationsEndpoint), portalAPIChain, http.HandlerFunc(s.getUserOrgs))
	rg.Handle(rg.Post(common.OrgEndpoint), portalAPIChain, http.MaxBytesHandler(http.HandlerFunc(s.postNewOrg), maxAPIPostBodySize))
	rg.Handle(rg.Put(common.OrgEndpoint), portalAPIChain, http.MaxBytesHandler(http.HandlerFunc(s.updateOrg), maxAPIPostBodySize))
	rg.Handle(rg.Delete(common.OrgEndpoint), portalAPIChain, http.MaxBytesHandler(http.HandlerFunc(s.deleteOrg), maxAPIPostBodySize))
}

func (s *Server) requestUser(ctx context.Context) (*dbgen.User, error) {
	ownerSource := &apiKeyOwnerSource{Store: s.BusinessDB, scope: dbgen.ApiKeyScopePortal}
	id, err := ownerSource.OwnerID(ctx, time.Now().UTC())
	if err != nil {
		return nil, err
	}

	return s.BusinessDB.Impl().RetrieveUser(ctx, id)
}

func (s *Server) sendHTTPErrorResponse(err error, w http.ResponseWriter) {
	switch err {
	case db.ErrRecordNotFound:
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
	case db.ErrInvalidInput:
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
	case db.ErrMaintenance:
		http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
	case errAPIKeyScope, errInvalidAPIKey, errAPIKeyNotSet:
		http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
	case db.ErrSoftDeleted:
		http.Error(w, http.StatusText(http.StatusConflict), http.StatusConflict)
	}
}

func (s *Server) sendAPISuccessResponse(ctx context.Context, data interface{}, w http.ResponseWriter) {
	response := &APIResponse{
		Meta: ResponseMetadata{
			Code:        common.StatusOK,
			Description: common.StatusOK.String(),
		},
		Data: data,
	}

	if tid, ok := ctx.Value(common.TraceIDContextKey).(string); ok {
		response.Meta.RequestID = tid
	}

	common.SendJSONResponse(ctx, w, response, common.NoCacheHeaders)
}

func (s *Server) sendAPIErrorResponse(ctx context.Context, code common.StatusCode, w http.ResponseWriter) {
	response := &APIResponse{
		Meta: ResponseMetadata{
			Code:        code,
			Description: code.String(),
		},
	}

	if tid, ok := ctx.Value(common.TraceIDContextKey).(string); ok {
		response.Meta.RequestID = tid
	}

	common.SendJSONResponse(ctx, w, response, common.NoCacheHeaders)
}
