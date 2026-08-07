package group

import (
	"context"
	"errors"

	groupDb "github.com/super-phenix/superphenix/internal/superphenix-api/internal/db/crud/group"
	orgaDb "github.com/super-phenix/superphenix/internal/superphenix-api/internal/db/crud/organization"
	"github.com/super-phenix/superphenix/internal/superphenix-api/internal/db/model"
	logger "github.com/super-phenix/superphenix/pkg/utils/log"

	v1 "github.com/super-phenix/superphenix/pkg/permify-wrapper/pkg/base/v1"
	"github.com/super-phenix/superphenix/pkg/permify-wrapper/pkg/schema"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ReconcileReport summarizes one reconciliation pass.
type ReconcileReport struct {
	OrganizationsScanned int `json:"organizationsScanned"`
	OrganizationsUpdated int `json:"organizationsUpdated"`
	GroupsCreated        int `json:"groupsCreated"`
	GroupsUpdated        int `json:"groupsUpdated"`
	// OrganizationFailedIds are retried on the next pass.
	OrganizationFailedIds []uuid.UUID `json:"organizationFailedIds"`
}

// ReconcilePredefinedGroups aligns every organization's predefined groups on v1.PredefinedGroups.
// Staleness is tracked per organization because a catalog change usually also changes the Permify
// schema, which has to be pushed before the groups. Custom groups carry no predefined key, so no
// catalog entry resolves to them.
func ReconcilePredefinedGroups(ctx context.Context, force bool) (ReconcileReport, error) {
	log := logger.GetLogger(ctx)
	report := ReconcileReport{OrganizationFailedIds: []uuid.UUID{}}

	organizations, err := orgaDb.FindStaleForPredefinedCatalog(v1.PredefinedCatalogVersion, force)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list organizations to reconcile")
		return report, err
	}

	report.OrganizationsScanned = len(organizations)
	if len(organizations) == 0 {
		return report, nil
	}

	log.Info().Int("organizations", len(organizations)).Bool("force", force).
		Int("catalogVersion", v1.PredefinedCatalogVersion).Msg("Reconciling predefined IAM groups")

	for _, orga := range organizations {
		created, updated, err := reconcileOrganization(ctx, orga.ID)
		report.GroupsCreated += created
		report.GroupsUpdated += updated

		if err != nil {
			// Version left untouched, so the organization is picked up again next time.
			report.OrganizationFailedIds = append(report.OrganizationFailedIds, orga.ID)
			log.Error().Err(err).Str("orgaId", orga.ID.String()).
				Msg("Failed to reconcile predefined groups, will retry on next pass")
			continue
		}

		if err := orgaDb.SetPredefinedCatalogVersion(orga.ID, v1.PredefinedCatalogVersion); err != nil {
			report.OrganizationFailedIds = append(report.OrganizationFailedIds, orga.ID)
			log.Error().Err(err).Str("orgaId", orga.ID.String()).
				Msg("Failed to record predefined catalog version")
			continue
		}
		report.OrganizationsUpdated++
	}

	return report, nil
}

// reconcileOrganization pushes the default schema then upserts every catalog entry.
func reconcileOrganization(ctx context.Context, orgaId uuid.UUID) (created, updated int, err error) {
	if err := schema.Write(ctx, orgaId.String(), v1.DefaultSchema); err != nil {
		return created, updated, err
	}

	for _, predefined := range v1.PredefinedGroups {
		group, err := groupDb.FindByPredefinedKey(orgaId, predefined.Key)
		switch {
		case err == nil:
			// SaveGroup re-pushes the Permify tuples with a delete-then-write, so removals apply.
			group.Name = predefined.Name
			group.PermissionSets = predefined.Sets()
			if _, err := SaveGroup(ctx, group); err != nil {
				return created, updated, err
			}
			updated++
		case errors.Is(err, gorm.ErrRecordNotFound):
			// New catalog entry, or an organization whose group the backfill left custom.
			key := predefined.Key
			if _, err := SaveGroup(ctx, model.Group{
				Name:           predefined.Name,
				OrgaId:         orgaId,
				AllProjects:    true,
				PermissionSets: predefined.Sets(),
				PredefinedKey:  &key,
			}); err != nil {
				return created, updated, err
			}
			created++
		default:
			return created, updated, err
		}
	}

	return created, updated, nil
}
