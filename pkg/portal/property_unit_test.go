package portal

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/common"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/config"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/db"
	dbgen "github.com/PrivateCaptcha/PrivateCaptcha/pkg/db/generated"
	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/puzzle"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestPropertyToUserPropertyMapsFields(t *testing.T) {
	hasher := common.NewIDHasher(config.NewStaticValue(common.IDHasherSaltKey, "salt"))
	property := &dbgen.Property{
		ID:               10,
		OrgID:            db.Int(20),
		Name:             "Example",
		Domain:           "example.com",
		Level:            db.Int2(3),
		Growth:           dbgen.DifficultyGrowthFast,
		ExternalID:       db.UUIDFromSiteKey(db.TestPropertySitekey),
		ValidityInterval: 2 * time.Hour,
		AllowSubdomains:  true,
		AllowLocalhost:   true,
		MaxReplayCount:   5,
	}

	up := propertyToUserProperty(property, hasher)

	if up.ID == strconv.Itoa(int(property.ID)) {
		t.Fatalf("expected hashed id, got plain %s", up.ID)
	}
	if up.OrgID == strconv.Itoa(int(property.OrgID.Int32)) {
		t.Fatalf("expected hashed org id, got plain %s", up.OrgID)
	}
	if up.Growth != 3 {
		t.Fatalf("unexpected growth index %d", up.Growth)
	}
	if up.Sitekey == "" {
		t.Fatalf("expected sitekey to be generated")
	}
	if up.ValidityInterval != puzzle.ValidityIntervalToIndex(property.ValidityInterval) {
		t.Fatalf("unexpected validity interval %d", up.ValidityInterval)
	}
	if !up.AllowReplay || up.MaxReplayCount != int(property.MaxReplayCount) {
		t.Fatalf("replay flags were not propagated")
	}
	if !up.AllowSubdomains || !up.AllowLocalhost {
		t.Fatalf("expected allow flags to be preserved")
	}
}

func TestPropertiesToUserPropertiesSkipsDeleted(t *testing.T) {
	hasher := common.NewIDHasher(config.NewStaticValue(common.IDHasherSaltKey, "salt"))
	active := &dbgen.Property{
		ID:               1,
		OrgID:            db.Int(2),
		Level:            db.Int2(1),
		Growth:           dbgen.DifficultyGrowthConstant,
		ExternalID:       db.UUIDFromSiteKey(db.TestPropertySitekey),
		ValidityInterval: time.Hour,
	}
	deleted := *active
	deleted.ID = 3
	deleted.DeletedAt = pgtype.Timestamptz{Valid: true, Time: time.Now()}

	props := propertiesToUserProperties(context.Background(), []*dbgen.Property{active, &deleted}, hasher)
	if len(props) != 1 {
		t.Fatalf("expected only active properties, got %d", len(props))
	}
	if props[0].ID == strconv.Itoa(int(active.ID)) {
		t.Fatalf("expected hashed id for active property")
	}
}

func TestPropertyParsingHelpers(t *testing.T) {
	ctx := context.Background()

	if val := growthLevelFromIndex(ctx, "3"); val != dbgen.DifficultyGrowthFast {
		t.Fatalf("unexpected growth value %v", val)
	}
	if val := growthLevelFromIndex(ctx, "invalid"); val != dbgen.DifficultyGrowthMedium {
		t.Fatalf("unexpected fallback growth value %v", val)
	}

	if count := parseMaxReplayCount(ctx, "2000000"); count != 1_000_000 {
		t.Fatalf("expected max replay count to be clamped, got %d", count)
	}
	if count := parseMaxReplayCount(ctx, "bad"); count != 1 {
		t.Fatalf("expected invalid replay count to fall back to 1, got %d", count)
	}

	if level := difficultyLevelFromValue(ctx, "5", 10, 20); level != 10 {
		t.Fatalf("expected difficulty to be clamped to min, got %d", level)
	}
	if level := difficultyLevelFromValue(ctx, "bad", 1, 255); level != common.DifficultyLevelMedium {
		t.Fatalf("expected invalid value to fall back to medium, got %d", level)
	}
}

func TestUpdateLevelsClampsPropertyLevel(t *testing.T) {
	property := &userProperty{Level: 200}
	renderCtx := &propertySettingsRenderContext{
		propertyDashboardRenderContext: propertyDashboardRenderContext{
			Property: property,
		},
		difficultyLevelsRenderContext: difficultyLevelsRenderContext{
			EasyLevel:   80,
			NormalLevel: 95,
			HardLevel:   110,
		},
	}

	renderCtx.UpdateLevels()

	if renderCtx.MaxLevel != 125 {
		t.Fatalf("unexpected max level %d", renderCtx.MaxLevel)
	}
	if renderCtx.MinLevel != 65 {
		t.Fatalf("unexpected min level %d", renderCtx.MinLevel)
	}
	if property.Level != renderCtx.MaxLevel {
		t.Fatalf("expected property level to be clamped to %d, got %d", renderCtx.MaxLevel, property.Level)
	}
}
