package access

import (
	"context"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/apperror"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/auth"
)

type Repository interface {
	ListUserGrants(ctx context.Context, userID, databaseID string) ([]api.AccessLevel, error)
	ListGrantedDatabases(ctx context.Context, userID string) ([]api.ManagedDatabaseSummary, error)
	GetDatabaseSummary(ctx context.Context, databaseID string) (api.ManagedDatabaseSummary, error)
}

type Service struct {
	Repository Repository
}

// RequirePlatformAdmin checks the current platform role.
func (s *Service) RequirePlatformAdmin(ctx context.Context, actor auth.Principal) error {
	// TODO Reject a missing identity and require the freshly validated platform admin flag.
	// TODO Keep this separate from target database permissions.
	return apperror.ErrNotImplemented
}

// EffectiveAccess calculates the strongest grant for one target.
func (s *Service) EffectiveAccess(ctx context.Context, actor auth.Principal, databaseID string) (api.AccessLevel, error) {
	// TODO Read current grants across every group on each protected operation.
	// TODO Return VIEW_EDIT if any grant allows edit and otherwise VIEW_ONLY if present.
	// TODO Return a forbidden error when no grant exists or the database is disabled.
	// TODO Do not grant target access just because the user is a platform admin.
	return "", apperror.ErrNotImplemented
}

// RequireDatabase enforces a minimum target permission.
func (s *Service) RequireDatabase(ctx context.Context, actor auth.Principal, databaseID string, required api.AccessLevel) error {
	// TODO Reject empty identities and unknown required levels before checking grants.
	// TODO Call EffectiveAccess and compare levels using an explicit ordering.
	// TODO Treat VIEW_EDIT as including VIEW_ONLY and deny any other case.
	return apperror.ErrNotImplemented
}

// ListDatabases returns only targets visible to the authenticated user.
func (s *Service) ListDatabases(ctx context.Context, actor auth.Principal) ([]api.UserDatabase, error) {
	// TODO Load distinct granted targets and use EffectiveAccess for each target.
	// TODO Map the existing summary fields into api.UserDatabase.
	return nil, apperror.ErrNotImplemented
}

// GetDatabase returns a permitted target summary.
func (s *Service) GetDatabase(ctx context.Context, actor auth.Principal, databaseID string) (api.UserDatabase, error) {
	// TODO Require at least VIEW_ONLY and return the current effective access.
	// TODO Return only safe fields from GetDatabaseSummary.
	return api.UserDatabase{}, apperror.ErrNotImplemented
}
