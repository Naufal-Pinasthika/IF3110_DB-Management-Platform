package access

import (
	"context"
	"errors"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/apperror"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/auth"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/models"
)

type Repository interface {
	ListUserGrants(ctx context.Context, userID, databaseID string) ([]models.GroupAccess, error)
	ListGrantedDatabases(ctx context.Context, userID string) ([]models.ManagedDatabase, error)
	GetDatabaseSummary(ctx context.Context, databaseID string) (models.ManagedDatabase, error)
}

type Service struct {
	Repository Repository
}

var (
	ErrUnauthenticated = errors.New("unauthenticated")
	ErrForbidden       = errors.New("forbidden")
	ErrNotFound        = errors.New("not found")
)

// checks the current platform role. rejects missing identity
func (s *Service) RequirePlatformAdmin(ctx context.Context, actor auth.Principal) error {
	if actor.User.ID == "" {
		return ErrUnauthenticated
	}
	if !actor.User.IsPlatformAdmin {
		return ErrForbidden
	}
	return nil
}

// calculates the strongest grant for one target.
func (s *Service) EffectiveAccess(ctx context.Context, actor auth.Principal, databaseID string) (api.AccessLevel, error) {
	if actor.User.ID == "" {
		return "", ErrUnauthenticated
	}

	grants, err := s.Repository.ListUserGrants(ctx, actor.User.ID, databaseID)
	if err != nil {
		return "", err
	}

	if len(grants) == 0 {
		return "", ErrForbidden
	}

	// VIEW_EDIT is stronger than VIEW_ONLY
	for _, grant := range grants {
		if grant.AccessLevel == models.AccessViewEdit {
			return api.AccessViewEdit, nil
		}
	}

	return api.AccessViewOnly, nil
}

// enforces a minimum target permission.
func (s *Service) RequireDatabase(ctx context.Context, actor auth.Principal, databaseID string, required api.AccessLevel) error {
	if actor.User.ID == "" {
		return ErrUnauthenticated
	}

	// Validate required level
	if required != api.AccessViewOnly && required != api.AccessViewEdit {
		return ErrForbidden
	}

	effective, err := s.EffectiveAccess(ctx, actor, databaseID)
	if err != nil {
		return err
	}

	if required == api.AccessViewEdit && effective != api.AccessViewEdit {
		return ErrForbidden
	}

	return nil
}

// returns only targets visible to the authenticated user.
func (s *Service) ListDatabases(ctx context.Context, actor auth.Principal) ([]api.UserDatabase, error) {
	if actor.User.ID == "" {
		return nil, ErrUnauthenticated
	}

	databases, err := s.Repository.ListGrantedDatabases(ctx, actor.User.ID)
	if err != nil {
		return nil, err
	}

	result := make([]api.UserDatabase, 0, len(databases))
	for _, db := range databases {
		effective, err := s.EffectiveAccess(ctx, actor, db.ID)
		if err != nil {	
			if errors.Is(err, ErrForbidden) {
				continue
			}
			return nil, err
		}

		result = append(result, api.UserDatabase{
			ID:               db.ID,
			Label:            db.Label,
			DatabaseName:     db.DatabaseName,
			EffectiveAccess:  effective,
		})
	}

	return result, nil
}

// returns a permitted target summary.
func (s *Service) GetDatabase(ctx context.Context, actor auth.Principal, databaseID string) (api.UserDatabase, error) {
	if actor.User.ID == "" {
		return api.UserDatabase{}, ErrUnauthenticated
	}

	effective, err := s.EffectiveAccess(ctx, actor, databaseID)
	if err != nil {
		return api.UserDatabase{}, err
	}

	db, err := s.Repository.GetDatabaseSummary(ctx, databaseID)
	if err != nil {
		if errors.Is(err, apperror.ErrNotImplemented) {
			return api.UserDatabase{}, ErrNotFound
		}
		return api.UserDatabase{}, err
	}

	return api.UserDatabase{
		ID:              db.ID,
		Label:           db.Label,
		DatabaseName:    db.DatabaseName,
		EffectiveAccess: effective,
	}, nil
}
