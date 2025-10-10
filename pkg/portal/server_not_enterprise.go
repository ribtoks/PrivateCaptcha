//go:build !enterprise

package portal

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/db"
	dbgen "github.com/PrivateCaptcha/PrivateCaptcha/pkg/db/generated"
	"github.com/justinas/alice"
)

func (s *Server) isEnterprise() bool {
	return false
}

// in not-EE environment user can only load the org they own
func (s *Server) checkUserOrgAccess(user *dbgen.User, org *dbgen.Organization) bool {
	return (user != nil) &&
		(org != nil) &&
		org.UserID.Valid &&
		(user.ID == org.UserID.Int32)
}

func (s *Server) checkUserOrgsLimit(ctx context.Context, user *dbgen.User, count int) bool {
	if count <= 1 {
		return true
	}

	if user.SubscriptionID.Valid {
		if subscription, err := s.Store.Impl().RetrieveSubscription(ctx, user.SubscriptionID.Int32); err == nil {
			if plan, err := s.PlanService.FindPlan(subscription.ExternalProductID, subscription.ExternalPriceID, s.Stage,
				db.IsInternalSubscription(subscription.Source)); err == nil {
				return plan.CheckOrgsLimit(count)
			}
		}
	} else {
		slog.DebugContext(ctx, "User subscription is not valid", "userID", user.ID)
		return false
	}

	return true
}

func (s *Server) setupEnterprise(*http.ServeMux, *RouteGenerator, alice.Chain) {
	// BUMP
}
