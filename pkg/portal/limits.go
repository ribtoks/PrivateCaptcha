package portal

import (
	"context"
	"errors"
	"log/slog"

	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/billing"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/common"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/db"
	dbgen "github.com/PrivateCaptcha/PrivateCaptcha/pkg/db/generated"
)

var (
	ErrNoActiveSubscription = errors.New("subscription is not active or nil")
)

type SubscriptionLimits interface {
	CheckOrgsLimit(ctx context.Context, userID int32, subscr *dbgen.Subscription) (bool, error)
	CheckOrgMembersLimit(ctx context.Context, orgID int32, subscr *dbgen.Subscription) (bool, error)
	CheckPropertiesLimit(ctx context.Context, userID int32, subscr *dbgen.Subscription) (bool, error)
	RequestsLimit(ctx context.Context, subscr *dbgen.Subscription) (int64, error)
	PropertiesLimit(ctx context.Context, subscr *dbgen.Subscription) (int, error)
	OrgsLimit(ctx context.Context, subscr *dbgen.Subscription) (int, error)
}

type SubscriptionLimitsImpl struct {
	stage       string
	store       db.Implementor
	planService billing.PlanService
}

func NewSubscriptionLimits(stage string, store db.Implementor, planService billing.PlanService) *SubscriptionLimitsImpl {
	return &SubscriptionLimitsImpl{
		stage:       stage,
		store:       store,
		planService: planService,
	}
}

var _ SubscriptionLimits = (*SubscriptionLimitsImpl)(nil)

func (sl *SubscriptionLimitsImpl) CheckOrgsLimit(ctx context.Context, userID int32, subscr *dbgen.Subscription) (bool, error) {
	if (subscr == nil) || !sl.planService.IsSubscriptionActive(subscr.Status) {
		return false, ErrNoActiveSubscription
	}

	isInternalSubscription := db.IsInternalSubscription(subscr.Source)
	plan, err := sl.planService.FindPlan(subscr.ExternalProductID, subscr.ExternalPriceID, sl.stage, isInternalSubscription)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to find billing plan for subscription", "subscriptionID", subscr.ID, common.ErrAttr(err))
		return false, err
	}

	count := 0
	total := 0
	// NOTE: this should be freshly cached as we should have just rendered the dashboard
	if orgs, err := sl.store.Impl().RetrieveUserOrganizations(ctx, userID); err == nil {
		total = len(orgs)
		for _, org := range orgs {
			if org.Level == dbgen.AccessLevelOwner {
				count++
			}
		}
	} else {
		{
			slog.ErrorContext(ctx, "Failed to retrieve user orgs", "userID", userID, common.ErrAttr(err))
			return false, err
		}
	}

	ok := (plan.OrgsLimit() == 0) || (count < plan.OrgsLimit())
	if !ok {
		slog.WarnContext(ctx, "Organizations limit check failed", "total", total, "owned", count, "userID", userID, "subscriptionID", subscr.ID,
			"plan", plan.Name(), "internal", isInternalSubscription)
	}

	return ok, nil
}

func (sl *SubscriptionLimitsImpl) CheckOrgMembersLimit(ctx context.Context, orgID int32, subscr *dbgen.Subscription) (bool, error) {
	if (subscr == nil) || !sl.planService.IsSubscriptionActive(subscr.Status) {
		return false, ErrNoActiveSubscription
	}

	isInternalSubscription := db.IsInternalSubscription(subscr.Source)
	plan, err := sl.planService.FindPlan(subscr.ExternalProductID, subscr.ExternalPriceID, sl.stage, isInternalSubscription)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to find billing plan for subscription", "subscriptionID", subscr.ID, common.ErrAttr(err))
		return false, err
	}

	members, err := sl.store.Impl().RetrieveOrganizationUsers(ctx, orgID)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to retrieve org users", common.ErrAttr(err))
		return false, err
	}

	ok := (plan.OrgMembersLimit() == 0) || (len(members) < plan.OrgMembersLimit())
	if !ok {
		slog.WarnContext(ctx, "Organization members limit check failed", "members", len(members), "orgID", orgID, "subscriptionID", subscr.ID,
			"plan", plan.Name(), "internal", isInternalSubscription)
	}

	return ok, nil
}

func (sl *SubscriptionLimitsImpl) CheckPropertiesLimit(ctx context.Context, userID int32, subscr *dbgen.Subscription) (bool, error) {
	if (subscr == nil) || !sl.planService.IsSubscriptionActive(subscr.Status) {
		return false, ErrNoActiveSubscription
	}

	isInternalSubscription := db.IsInternalSubscription(subscr.Source)
	plan, err := sl.planService.FindPlan(subscr.ExternalProductID, subscr.ExternalPriceID, sl.stage, isInternalSubscription)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to find billing plan for subscription", "subscriptionID", subscr.ID, common.ErrAttr(err))
		return false, err
	}

	count, err := sl.store.Impl().RetrieveUserPropertiesCount(ctx, userID)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to retrieve properties count", "userID", userID, common.ErrAttr(err))
		return false, err
	}

	ok := (plan.PropertiesLimit() == 0) || (count < int64(plan.PropertiesLimit()))
	if !ok {
		slog.WarnContext(ctx, "Properties limit check failed", "properties", count, "userID", userID, "subscriptionID", subscr.ID,
			"plan", plan.Name(), "internal", isInternalSubscription)
	}

	return ok, nil
}

func (sl *SubscriptionLimitsImpl) RequestsLimit(ctx context.Context, subscr *dbgen.Subscription) (int64, error) {
	if (subscr == nil) || !sl.planService.IsSubscriptionActive(subscr.Status) {
		return 0, ErrNoActiveSubscription
	}

	plan, err := sl.planService.FindPlan(subscr.ExternalProductID, subscr.ExternalPriceID, sl.stage,
		db.IsInternalSubscription(subscr.Source))
	if err != nil {
		slog.ErrorContext(ctx, "Failed to find billing plan", "productID", subscr.ExternalProductID, "priceID", subscr.ExternalPriceID, common.ErrAttr(err))
		return 0, err

	}
	return plan.RequestsLimit(), nil
}

func (sl *SubscriptionLimitsImpl) PropertiesLimit(ctx context.Context, subscr *dbgen.Subscription) (int, error) {
	if (subscr == nil) || !sl.planService.IsSubscriptionActive(subscr.Status) {
		return 0, ErrNoActiveSubscription
	}

	plan, err := sl.planService.FindPlan(subscr.ExternalProductID, subscr.ExternalPriceID, sl.stage,
		db.IsInternalSubscription(subscr.Source))
	if err != nil {
		slog.ErrorContext(ctx, "Failed to find billing plan", "productID", subscr.ExternalProductID, "priceID", subscr.ExternalPriceID, common.ErrAttr(err))
		return 0, err

	}
	return plan.PropertiesLimit(), nil
}

func (sl *SubscriptionLimitsImpl) OrgsLimit(ctx context.Context, subscr *dbgen.Subscription) (int, error) {
	if (subscr == nil) || !sl.planService.IsSubscriptionActive(subscr.Status) {
		return 0, ErrNoActiveSubscription
	}

	plan, err := sl.planService.FindPlan(subscr.ExternalProductID, subscr.ExternalPriceID, sl.stage,
		db.IsInternalSubscription(subscr.Source))
	if err != nil {
		slog.ErrorContext(ctx, "Failed to find billing plan", "productID", subscr.ExternalProductID, "priceID", subscr.ExternalPriceID, common.ErrAttr(err))
		return 0, err

	}
	return plan.OrgsLimit(), nil
}

type StubSubscriptionLimits struct{}

func (StubSubscriptionLimits) CheckOrgsLimit(ctx context.Context, userID int32, subscr *dbgen.Subscription) (_ bool, _ error) {
	return true, nil
}
func (StubSubscriptionLimits) CheckOrgMembersLimit(ctx context.Context, orgID int32, subscr *dbgen.Subscription) (_ bool, _ error) {
	return true, nil
}
func (StubSubscriptionLimits) CheckPropertiesLimit(ctx context.Context, userID int32, subscr *dbgen.Subscription) (_ bool, _ error) {
	return true, nil
}
func (StubSubscriptionLimits) RequestsLimit(ctx context.Context, subscr *dbgen.Subscription) (int64, error) {
	return 0, nil
}
func (StubSubscriptionLimits) PropertiesLimit(ctx context.Context, subscr *dbgen.Subscription) (int, error) {
	return 0, nil
}
func (StubSubscriptionLimits) OrgsLimit(ctx context.Context, subscr *dbgen.Subscription) (int, error) {
	return 0, nil
}

var _ SubscriptionLimits = (*StubSubscriptionLimits)(nil)
