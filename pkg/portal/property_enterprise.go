//go:build enterprise

package portal

import (
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/common"
	dbgen "github.com/PrivateCaptcha/PrivateCaptcha/pkg/db/generated"
)

func (s *Server) moveProperty(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	err := r.ParseForm()
	if err != nil {
		slog.ErrorContext(ctx, "Failed to read request body", common.ErrAttr(err))
		s.RedirectError(http.StatusBadRequest, w, r)
		return
	}

	newOrgParam := strings.TrimSpace(r.FormValue(common.ParamOrg))
	newOrgID, err := strconv.Atoi(newOrgParam)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to parse new org ID", "value", newOrgParam, common.ErrAttr(err))
		s.RedirectError(http.StatusBadRequest, w, r)
		return
	}

	user, err := s.SessionUser(ctx, s.Session(w, r))
	if err != nil {
		s.RedirectError(http.StatusUnauthorized, w, r)
		return
	}

	org, err := s.Org(user, r)
	if err != nil {
		s.RedirectError(http.StatusInternalServerError, w, r)
		return
	}

	if org.ID == int32(newOrgID) {
		// this shouldn't happen as we don't expose such option in FE
		slog.ErrorContext(ctx, "Attempt to move property to the same org", "orgID", newOrgID)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	property, err := s.Property(org, r)
	if err != nil {
		s.RedirectError(http.StatusBadRequest, w, r)
		return
	}

	// we can only move properties that we created
	canMove := user.ID == property.CreatorID.Int32
	if !canMove {
		slog.ErrorContext(ctx, "Not enough permissions to move property", "userID", user.ID,
			"orgUserID", org.UserID.Int32, "propertyUserID", property.CreatorID.Int32)
		s.RedirectError(http.StatusUnauthorized, w, r)
		return
	}

	orgs, err := s.Store.Impl().RetrieveUserOrganizations(ctx, user)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to retrieve user orgs", common.ErrAttr(err))
		s.RedirectError(http.StatusInternalServerError, w, r)
		return
	}

	idx := slices.IndexFunc(orgs, func(o *dbgen.GetUserOrganizationsRow) bool {
		return (o.Organization.ID == int32(newOrgID)) && (o.Level == dbgen.AccessLevelOwner)
	})
	if idx == -1 {
		slog.ErrorContext(ctx, "Org is not found in user owned orgs", "orgID", newOrgID, "userID", user.ID)
		s.RedirectError(http.StatusBadRequest, w, r)
		return
	}

	if updatedProperty, err := s.Store.Impl().MoveProperty(ctx, property, orgs[idx]); err == nil {
		propertyDashboardURL := s.PartsURL(common.OrgEndpoint, strconv.Itoa(int(updatedProperty.OrgID.Int32)), common.PropertyEndpoint, strconv.Itoa(int(updatedProperty.ID)))
		common.Redirect(propertyDashboardURL, http.StatusOK, w, r)
	} else {
		s.RedirectError(http.StatusInternalServerError, w, r)
	}
}
