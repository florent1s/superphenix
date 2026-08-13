package group

import (
	"context"
	"testing"

	"github.com/super-phenix/superphenix/internal/superphenix-api/internal/db"
	"github.com/super-phenix/superphenix/internal/superphenix-api/internal/db/crud/quota"
	httpModel "github.com/super-phenix/superphenix/internal/superphenix-api/pkg/api/publicHttp/model"

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

	gormDB, err := gorm.Open(postgres.New(postgres.Config{
		Conn: sqlDB,
	}), &gorm.Config{})
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

func TestFindByPredefinedKey(t *testing.T) {
	orgaId := uuid.New()
	groupId := uuid.New()

	tests := []struct {
		name     string
		key      string
		rows     *sqlmock.Rows
		wantErr  bool
		wantName string
	}{
		{
			name: "returns the group holding the key",
			key:  v1.PredefinedGroupAdmin,
			rows: sqlmock.NewRows([]string{"id", "name", "orga_id", "all_projects", "project_ids", "permission_sets", "predefined_key"}).
				AddRow(groupId, "Admin", orgaId, true, "[]", "[]", "admin"),
			wantName: "Admin",
		},
		{
			name:    "errors when the organization has no group for the key",
			key:     v1.PredefinedGroupBilling,
			rows:    sqlmock.NewRows([]string{"id", "name", "orga_id", "all_projects", "project_ids", "permission_sets", "predefined_key"}),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock, cleanup := setupMockDB(t)
			defer cleanup()

			mock.ExpectQuery(`predefined_key`).WillReturnRows(tt.rows)

			result, err := FindByPredefinedKey(orgaId, tt.key)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.wantName, result.Name)
			assert.NotNil(t, result.PredefinedKey)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// The owner group is identified by its catalog key, so renaming it does not hide it.
func TestOwnerLookupsUsePredefinedKey(t *testing.T) {
	orgaId := uuid.New()

	tests := []struct {
		name  string
		query func() error
	}{
		{
			name: "FindOwnerGroup",
			query: func() error {
				_, err := FindOwnerGroup(orgaId.String())
				return err
			},
		},
		{
			name: "FindAllByOrgaIdExceptOwner",
			query: func() error {
				_, err := FindAllByOrgaIdExceptOwner(orgaId.String())
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock, cleanup := setupMockDB(t)
			defer cleanup()

			// The expectation asserts the filter: a name-based query would not match.
			mock.ExpectQuery(`predefined_key`).WillReturnRows(
				sqlmock.NewRows([]string{"id", "name", "orga_id", "all_projects", "project_ids", "permission_sets", "predefined_key"}),
			)

			_ = tt.query()
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// Predefined groups must not consume the user's allowance.
func TestIsQuotaCreationReachedIgnoresPredefinedGroups(t *testing.T) {
	orgaId := uuid.New()

	tests := []struct {
		name         string
		customGroups int64
		limit        string
		want         bool
	}{
		{name: "below the limit", customGroups: 3, limit: "15", want: false},
		{name: "at the limit", customGroups: 15, limit: "15", want: true},
		{name: "above the limit", customGroups: 16, limit: "15", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock, cleanup := setupMockDB(t)
			defer cleanup()

			// No override, so the default quota is used.
			mock.ExpectQuery(`quota_overrides`).WillReturnError(gorm.ErrRecordNotFound)
			mock.ExpectQuery(`FROM "quota"`).WillReturnRows(
				sqlmock.NewRows([]string{"id", "value"}).AddRow(quota.OrgaLimitIAMGroup, tt.limit),
			)
			// The expectation asserts the scoping: an unscoped count would not match.
			mock.ExpectQuery(`predefined_key IS NULL`).WillReturnRows(
				sqlmock.NewRows([]string{"count"}).AddRow(tt.customGroups),
			)

			reached, err := IsQuotaCreationReached(context.Background(), orgaId)
			assert.NoError(t, err)
			assert.Equal(t, tt.want, reached)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestFindAllByOrgaIdExceptOwner(t *testing.T) {
	orgaId := uuid.New()
	groupId1 := uuid.New()
	groupId2 := uuid.New()

	tests := []struct {
		name        string
		orgaId      string
		mockSetup   func(mock sqlmock.Sqlmock)
		wantCount   int
		wantErr     bool
		checkResult func(t *testing.T, result []httpModel.APIGroup)
	}{
		{
			name:   "returns groups excluding owner",
			orgaId: orgaId.String(),
			mockSetup: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id", "name", "orga_id", "all_projects", "project_ids", "permission_sets"}).
					AddRow(groupId1, "Developers", orgaId, false, "[]", "[]").
					AddRow(groupId2, "Viewers", orgaId, false, "[]", "[]")
				mock.ExpectQuery(`SELECT`).WillReturnRows(rows)
			},
			wantCount: 2,
			checkResult: func(t *testing.T, result []httpModel.APIGroup) {
				assert.Equal(t, "Developers", result[0].Name)
				assert.Equal(t, "Viewers", result[1].Name)
			},
		},
		{
			name:   "returns empty slice when no groups",
			orgaId: orgaId.String(),
			mockSetup: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id", "name", "orga_id", "all_projects", "project_ids", "permission_sets"})
				mock.ExpectQuery(`SELECT`).WillReturnRows(rows)
			},
			wantCount: 0,
			checkResult: func(t *testing.T, result []httpModel.APIGroup) {
				assert.NotNil(t, result, "result should be non-nil empty slice")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock, cleanup := setupMockDB(t)
			defer cleanup()

			tt.mockSetup(mock)

			result, err := FindAllByOrgaIdExceptOwner(tt.orgaId)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Len(t, result, tt.wantCount)
			if tt.checkResult != nil {
				tt.checkResult(t, result)
			}
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
