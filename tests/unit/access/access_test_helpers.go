package access

import (
	"context"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/auth"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/models"
)

// mockRepository implements Repository for testing
type mockRepository struct {
	grants       []models.GroupAccess
	databases    []models.ManagedDatabase
	dbSummary    models.ManagedDatabase
	grantsErr    error
	databasesErr error
	summaryErr   error
}

func (m *mockRepository) ListUserGrants(ctx context.Context, userID, databaseID string) ([]models.GroupAccess, error) {
	// Filter grants by databaseID
	filtered := make([]models.GroupAccess, 0)
	for _, grant := range m.grants {
		if grant.DatabaseID == databaseID {
			filtered = append(filtered, grant)
		}
	}
	return filtered, m.grantsErr
}

func (m *mockRepository) ListGrantedDatabases(ctx context.Context, userID string) ([]models.ManagedDatabase, error) {
	return m.databases, m.databasesErr
}

func (m *mockRepository) GetDatabaseSummary(ctx context.Context, databaseID string) (models.ManagedDatabase, error) {
	return m.dbSummary, m.summaryErr
}

func makePrincipal(userID string, isAdmin bool) auth.Principal {
	return auth.Principal{
		User: api.LoginResponse{
			ID:              userID,
			Username:        "testuser",
			IsPlatformAdmin: isAdmin,
		},
		SessionID: "session-123",
	}
}

func makeGrant(groupID, dbID string, level models.AccessLevel) models.GroupAccess {
	return models.GroupAccess{
		GroupID:     groupID,
		DatabaseID:  dbID,
		AccessLevel: level,
	}
}

func makeDatabase(id, label, name string) models.ManagedDatabase {
	return models.ManagedDatabase{
		ID:            id,
		Label:         label,
		DatabaseName:  name,
		IsEnabled:     true,
	}
}