package group

import (
	"context"
	"testing"

	"github.com/super-phenix/superphenix/internal/superphenix-api/internal/db"

	v1 "github.com/super-phenix/superphenix/pkg/permify-wrapper/pkg/base/v1"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func setupMockDB(t *testing.T) (sqlmock.Sqlmock, func()) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}

	gormDB, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open gorm: %v", err)
	}

	oldClient := db.Client
	db.Client = gormDB
	return mock, func() {
		db.Client = oldClient
		sqlDB.Close()
	}
}

func organizationRows(ids ...uuid.UUID) *sqlmock.Rows {
	rows := sqlmock.NewRows([]string{"id", "name", "owner_id", "predefined_catalog_version"})
	for i, id := range ids {
		rows.AddRow(id, "Org", uuid.New(), i)
	}
	return rows
}

func TestReconcilePredefinedGroups(t *testing.T) {
	orgaId := uuid.New()

	tests := []struct {
		name        string
		force       bool
		mockSetup   func(mock sqlmock.Sqlmock)
		wantScanned int
		wantUpdated int
		wantFailed  int
	}{
		{
			name: "no stale organization is a no-op",
			mockSetup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`predefined_catalog_version <`).WillReturnRows(organizationRows())
			},
			wantScanned: 0,
			wantUpdated: 0,
			wantFailed:  0,
		},
		{
			name:  "force selects every organization",
			force: true,
			mockSetup: func(mock sqlmock.Sqlmock) {
				// No version predicate. Permify is unavailable in tests, so the organization
				// then fails.
				mock.ExpectQuery(`FROM "organizations"`).WillReturnRows(organizationRows(orgaId))
			},
			wantScanned: 1,
			wantUpdated: 0,
			wantFailed:  1,
		},
		{
			// The absence of an UPDATE expectation is the assertion.
			name: "a failing organization keeps its catalog version",
			mockSetup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`predefined_catalog_version <`).WillReturnRows(organizationRows(orgaId))
			},
			wantScanned: 1,
			wantUpdated: 0,
			wantFailed:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock, cleanup := setupMockDB(t)
			defer cleanup()

			tt.mockSetup(mock)

			report, err := ReconcilePredefinedGroups(context.Background(), tt.force)
			assert.NoError(t, err)
			assert.Equal(t, tt.wantScanned, report.OrganizationsScanned)
			assert.Equal(t, tt.wantUpdated, report.OrganizationsUpdated)
			assert.Len(t, report.OrganizationFailedIds, tt.wantFailed)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// Custom groups carry no key, so no catalog entry resolves to one.
func TestReconcileOnlyTargetsCatalogKeys(t *testing.T) {
	for _, group := range v1.PredefinedGroups {
		assert.NotEmpty(t, group.Key)
		_, ok := v1.FindPredefinedGroup(group.Key)
		assert.True(t, ok)
	}
}
