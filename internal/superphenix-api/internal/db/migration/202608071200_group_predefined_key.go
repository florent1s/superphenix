package migration

import (
	"slices"
	"time"

	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// isAdoptable202608071200 reports whether a group can adopt a predefined key.
// Every permission set it holds must exist in the catalog. Holding fewer sets
// is fine, since older organizations predate some of them. Holding sets outside
// the catalog means someone customized the group, so it stays custom.
func isAdoptable202608071200(candidate, catalog []string) bool {
	for _, pSet := range candidate {
		if !slices.Contains(catalog, pSet) {
			return false
		}
	}
	return true
}

// migration202608071200 adds groups.predefined_key and organizations.predefined_catalog_version,
// then adopts the existing default groups into the catalog.
var migration202608071200 = &gormigrate.Migration{
	ID: "202608071200_group_predefined_key",
	Migrate: func(tx *gorm.DB) error {
		// Structs and permission sets are copied here so later catalog changes don't alter this
		// migration.
		type group struct {
			ID             uuid.UUID `gorm:"primaryKey;type:uuid"`
			CreatedAt      time.Time
			OrgaId         uuid.UUID
			Name           string
			PermissionSets []string `gorm:"serializer:json"`
			PredefinedKey  *string  `gorm:"index"`
		}

		type predefinedGroup struct {
			key            string
			name           string
			permissionSets []string
		}

		const (
			spxOwner                      = "spx_owner"
			spxMember                     = "spx_member"
			iamFullAccess                 = "IAMFullAccess"
			settingsEdition               = "SettingsEdition"
			billingFullAccess             = "BillingFullAccess"
			projectManagement             = "ProjectManagement"
			projectInstanceFullAccess     = "ProjectInstanceFullAccess"
			projectDiskFullAccess         = "ProjectDiskFullAccess"
			projectSnapshotFullAccess     = "ProjectSnapshotFullAccess"
			projectVPCFullAccess          = "ProjectVPCFullAccess"
			projectSubnetFullAccess       = "ProjectSubnetFullAccess"
			projectEipFullAccess          = "ProjectEipFullAccess"
			projectLoadBalancerFullAccess = "ProjectLoadBalancerFullAccess"
			projectFirewallFullAccess     = "ProjectFirewallFullAccess"
			projectSSHFullAccess          = "ProjectSSHFullAccess"
			projectKaaSFullAccess         = "ProjectKaaSFullAccess"
			projectBaaSFullAccess         = "ProjectBaaSFullAccess"
			projectBucketFullAccess       = "ProjectBucketFullAccess"
			projectArgoCdAccess           = "ProjectArgoCdAccess"
		)

		projectFullAccess := []string{
			projectInstanceFullAccess,
			projectDiskFullAccess,
			projectSnapshotFullAccess,
			projectVPCFullAccess,
			projectSubnetFullAccess,
			projectEipFullAccess,
			projectLoadBalancerFullAccess,
			projectFirewallFullAccess,
			projectSSHFullAccess,
			projectKaaSFullAccess,
			projectBaaSFullAccess,
			projectBucketFullAccess,
			projectArgoCdAccess,
		}

		catalog := []predefinedGroup{
			{key: "owner", name: "Owner", permissionSets: []string{spxOwner}},
			{key: "admin", name: "Admin", permissionSets: append([]string{
				spxMember,
				iamFullAccess,
				settingsEdition,
				billingFullAccess,
				projectManagement,
			}, projectFullAccess...)},
			{key: "billing", name: "Billing", permissionSets: []string{spxMember, billingFullAccess}},
			{key: "developer", name: "Developer", permissionSets: append([]string{
				spxMember,
				projectManagement,
			}, projectFullAccess...)},
		}

		// 1. Add both columns.
		if err := tx.Exec("ALTER TABLE groups ADD COLUMN IF NOT EXISTS predefined_key text").Error; err != nil {
			return err
		}
		if err := tx.Exec("CREATE INDEX IF NOT EXISTS idx_groups_predefined_key ON groups (predefined_key)").Error; err != nil {
			return err
		}
		if err := tx.Exec("ALTER TABLE organizations ADD COLUMN IF NOT EXISTS predefined_catalog_version bigint NOT NULL DEFAULT 0").Error; err != nil {
			return err
		}

		// 2. Adopt the existing default groups.
		for _, predefined := range catalog {
			var candidates []group
			// Oldest first, so that on two candidates in one organization the original wins.
			// The local struct has no DeletedAt, so the soft-delete filter is spelled out.
			if err := tx.Where("name = ? AND predefined_key IS NULL AND deleted_at IS NULL", predefined.name).
				Order("orga_id, created_at").
				Find(&candidates).Error; err != nil {
				return err
			}

			adoptedOrgs := make(map[uuid.UUID]struct{})
			for _, candidate := range candidates {
				if _, taken := adoptedOrgs[candidate.OrgaId]; taken {
					continue
				}

				if !isAdoptable202608071200(candidate.PermissionSets, predefined.permissionSets) {
					log.Info().
						Str("groupId", candidate.ID.String()).
						Str("orgaId", candidate.OrgaId.String()).
						Str("predefinedKey", predefined.key).
						Msg("Group holds permission sets outside the catalog, keeping it custom")
					continue
				}

				key := predefined.key
				if err := tx.Model(&group{}).Where("id = ?", candidate.ID).
					Update("predefined_key", &key).Error; err != nil {
					return err
				}
				adoptedOrgs[candidate.OrgaId] = struct{}{}
			}
		}

		// 3. One group per catalog entry per organization. Created after the backfill so a
		// duplicate fails the migration.
		if err := tx.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_groups_orga_predefined_key
			ON groups (orga_id, predefined_key)
			WHERE predefined_key IS NOT NULL AND deleted_at IS NULL`).Error; err != nil {
			return err
		}

		return nil
	},
	Rollback: func(tx *gorm.DB) error {
		log.Info().Msgf("rolling back migration 202608071200")

		if err := tx.Exec("DROP INDEX IF EXISTS idx_groups_orga_predefined_key").Error; err != nil {
			return err
		}
		if err := tx.Exec("DROP INDEX IF EXISTS idx_groups_predefined_key").Error; err != nil {
			return err
		}
		if err := tx.Exec("ALTER TABLE groups DROP COLUMN IF EXISTS predefined_key").Error; err != nil {
			return err
		}
		if err := tx.Exec("ALTER TABLE organizations DROP COLUMN IF EXISTS predefined_catalog_version").Error; err != nil {
			return err
		}

		return nil
	},
}
