//go:build enterprise

package portal

import (
	"context"
	"fmt"
	"net/http"

	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/common"
	dbgen "github.com/PrivateCaptcha/PrivateCaptcha/pkg/db/generated"
	"github.com/justinas/alice"
)

func (s *Server) isEnterprise() bool {
	return true
}

func (s *Server) checkUserOrgAccess(user *dbgen.User, org *dbgen.Organization) bool {
	// NOTE: actual org ownership permissions are correctly checked in s.Org()
	return true
}

func (s *Server) checkUserOrgsLimit(ctx context.Context, user *dbgen.User, count int) bool {
	return true
}

func (s *Server) setupEnterprise(router *http.ServeMux, rg *RouteGenerator, privateWrite alice.Chain) {
	arg := func(s string) string {
		return fmt.Sprintf("{%s}", s)
	}

	router.Handle(rg.Post(common.OrgEndpoint, common.NewEndpoint), privateWrite.ThenFunc(s.postNewOrg))
	router.Handle(rg.Post(common.OrgEndpoint, arg(common.ParamOrg), common.MembersEndpoint), privateWrite.Then(s.Handler(s.postOrgMembers)))
	router.Handle(rg.Delete(common.OrgEndpoint, arg(common.ParamOrg), common.MembersEndpoint, arg(common.ParamUser)), privateWrite.ThenFunc(s.deleteOrgMembers))
	router.Handle(rg.Put(common.OrgEndpoint, arg(common.ParamOrg), common.MembersEndpoint), privateWrite.ThenFunc(s.joinOrg))
	router.Handle(rg.Delete(common.OrgEndpoint, arg(common.ParamOrg), common.MembersEndpoint), privateWrite.ThenFunc(s.leaveOrg))
	router.Handle(rg.Delete(common.OrgEndpoint, arg(common.ParamOrg), common.DeleteEndpoint), privateWrite.ThenFunc(s.deleteOrg))
}
