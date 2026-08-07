// Package app holds the infrastructure bootstrap (database, Permify) shared by the API.
package app

import (
	"context"
	"sync"

	"github.com/super-phenix/superphenix/internal/superphenix-api/internal/db"
	"github.com/super-phenix/superphenix/internal/superphenix-api/pkg/config"
	"github.com/super-phenix/superphenix/internal/superphenix-api/pkg/services/iam/group"

	pwClient "github.com/super-phenix/superphenix/pkg/permify-wrapper/pkg/client"

	"github.com/rs/zerolog/log"
)

var (
	infraOnce sync.Once
	infraErr  error
)

// ProvideInfra connects the Permify and database clients. It is idempotent: the
// connection and migration run once regardless of how many composition roots
// (public, admin) call it, and every caller sees the same result. These
// initialisers set the package globals the current CRUD/permify helpers use.
func ProvideInfra(cfg *config.Config) error {
	infraOnce.Do(func() {
		if err := pwClient.InitPermify(cfg.Permify.Url); err != nil {
			infraErr = err
			return
		}
		if err := db.InitDatabase(
			cfg.Database.Host,
			cfg.Database.Username,
			cfg.Database.Password,
			cfg.Database.Database,
			cfg.Database.Port,
		); err != nil {
			infraErr = err
			return
		}

		// Runs after InitDatabase, which applies the migrations. Never fatal: the catalog version
		// is only recorded on success, so a failure is retried on the next boot.
		if report, err := group.ReconcilePredefinedGroups(context.Background(), false); err != nil {
			log.Error().Err(err).Msg("Failed to reconcile predefined IAM groups")
		} else if report.OrganizationsScanned > 0 {
			log.Info().
				Int("organizationsUpdated", report.OrganizationsUpdated).
				Int("groupsCreated", report.GroupsCreated).
				Int("groupsUpdated", report.GroupsUpdated).
				Int("organizationsFailed", len(report.OrganizationFailedIds)).
				Msg("Reconciled predefined IAM groups")
		}
	})
	return infraErr
}
