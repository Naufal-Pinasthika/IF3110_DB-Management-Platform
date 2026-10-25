package admin

import (
	"context"
	"time"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/apperror"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/auth"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/models"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/store"
)

type Repository interface {
	ListUsers(ctx context.Context) ([]models.User, error)
	GetUser(ctx context.Context, userID string) (models.User, error)
	CreateUser(ctx context.Context, user models.User) (models.User, error)
	UpdateUser(ctx context.Context, user models.User, revokeSessions bool, now time.Time) (models.User, error)
	ListGroups(ctx context.Context) ([]models.Group, error)
	CreateGroup(ctx context.Context, name string) (models.Group, error)
	UpdateGroup(ctx context.Context, groupID, name string) (models.Group, error)
	DeleteGroup(ctx context.Context, groupID string, now time.Time) error
	AddGroupMember(ctx context.Context, groupID, userID string, now time.Time) (models.UserGroup, error)
	RemoveGroupMember(ctx context.Context, groupID, userID string, now time.Time) error
	ListDatabases(ctx context.Context) ([]models.ManagedDatabase, error)
	GetDatabase(ctx context.Context, databaseID string) (models.ManagedDatabase, error)
	CreateDatabase(ctx context.Context, database models.ManagedDatabase) (models.ManagedDatabase, error)
	UpdateDatabase(ctx context.Context, database models.ManagedDatabase) (models.ManagedDatabase, error)
	DisableDatabase(ctx context.Context, databaseID string) error
	SetGroupDatabaseAccess(ctx context.Context, grant models.GroupAccess, now time.Time) (models.GroupAccess, error)
	DeleteGroupDatabaseAccess(ctx context.Context, groupID, databaseID string, now time.Time) error
}

type AdminChecker interface {
	RequirePlatformAdmin(context.Context, auth.Principal) error
}

type ConnectionTester interface {
	TestConnection(context.Context, string) error
}

// Service maps persistence models to the existing API response types.
type Service struct {
	Repository   Repository
	Access       AdminChecker
	Cipher       store.CredentialCipher
	Connections  ConnectionTester
	HashPassword func(string) (string, error)
}

// ListUsers lists platform accounts.
func (s *Service) ListUsers(ctx context.Context, actor auth.Principal) ([]api.AdminUser, error) {
	if err := s.Access.RequirePlatformAdmin(ctx, actor); err != nil {
		return nil, err
	}
	users, err := s.Repository.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]api.AdminUser, len(users))
	for i, u := range users {
		result[i] = api.AdminUser{
			ID:              u.ID,
			Username:        u.Username,
			IsPlatformAdmin: u.IsPlatformAdmin,
			IsActive:        u.IsActive,
		}
	}
	return result, nil
}

// CreateUser creates an account with a password hash.
func (s *Service) CreateUser(ctx context.Context, actor auth.Principal, input api.CreateUserRequest) (api.AdminUser, error) {
	if err := s.Access.RequirePlatformAdmin(ctx, actor); err != nil {
		return api.AdminUser{}, err
	}
	if input.Username == "" || input.Password == "" {
		return api.AdminUser{}, apperror.New("username and password required")
	}
	if len(input.Password) < 8 {
		return api.AdminUser{}, apperror.New("password must be at least 8 characters")
	}

	hash, err := s.HashPassword(input.Password)
	if err != nil {
		return api.AdminUser{}, err
	}

	user := models.User{
		Username:     input.Username,
		PasswordHash: hash,
		IsPlatformAdmin: input.IsPlatformAdmin,
		IsActive:     true,
	}

	created, err := s.Repository.CreateUser(ctx, user)
	if err != nil {
		return api.AdminUser{}, err
	}

	return api.AdminUser{
		ID:              created.ID,
		Username:        created.Username,
		IsPlatformAdmin: created.IsPlatformAdmin,
		IsActive:        created.IsActive,
	}, nil
}

// UpdateUser applies only supplied user fields.
func (s *Service) UpdateUser(ctx context.Context, actor auth.Principal, userID string, input api.UpdateUserRequest, now time.Time) (api.AdminUser, error) {
	if err := s.Access.RequirePlatformAdmin(ctx, actor); err != nil {
		return api.AdminUser{}, err
	}
	if userID == "" {
		return api.AdminUser{}, apperror.New("user id required")
	}
	if input.Username == nil && input.Password == nil && input.IsActive == nil {
		return api.AdminUser{}, apperror.New("no fields to update")
	}

	current, err := s.Repository.GetUser(ctx, userID)
	if err != nil {
		return api.AdminUser{}, err
	}

	revokeSessions := false
	if input.Username != nil {
		current.Username = *input.Username
	}
	if input.Password != nil {
		if len(*input.Password) < 8 {
			return api.AdminUser{}, apperror.New("password must be at least 8 characters")
		}
		hash, err := s.HashPassword(*input.Password)
		if err != nil {
			return api.AdminUser{}, err
		}
		current.PasswordHash = hash
		revokeSessions = true
	}
	if input.IsActive != nil {
		if !*input.IsActive {
			revokeSessions = true
		}
		current.IsActive = *input.IsActive
	}

	updated, err := s.Repository.UpdateUser(ctx, current, revokeSessions, now)
	if err != nil {
		return api.AdminUser{}, err
	}

	return api.AdminUser{
		ID:              updated.ID,
		Username:        updated.Username,
		IsPlatformAdmin: updated.IsPlatformAdmin,
		IsActive:        updated.IsActive,
	}, nil
}

// ListGroups lists platform groups.
func (s *Service) ListGroups(ctx context.Context, actor auth.Principal) ([]api.Group, error) {
	if err := s.Access.RequirePlatformAdmin(ctx, actor); err != nil {
		return nil, err
	}
	groups, err := s.Repository.ListGroups(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]api.Group, len(groups))
	for i, g := range groups {
		result[i] = api.Group{ID: g.ID, Name: g.Name}
	}
	return result, nil
}

// CreateGroup creates a named group.
func (s *Service) CreateGroup(ctx context.Context, actor auth.Principal, input api.CreateGroupRequest) (api.Group, error) {
	if err := s.Access.RequirePlatformAdmin(ctx, actor); err != nil {
		return api.Group{}, err
	}
	if input.Name == "" || len(input.Name) > 100 {
		return api.Group{}, apperror.New("group name required, max 100 characters")
	}

	group, err := s.Repository.CreateGroup(ctx, input.Name)
	if err != nil {
		return api.Group{}, err
	}
	return api.Group{ID: group.ID, Name: group.Name}, nil
}

// UpdateGroup updates a group name.
func (s *Service) UpdateGroup(ctx context.Context, actor auth.Principal, groupID string, input api.UpdateGroupRequest) (api.Group, error) {
	if err := s.Access.RequirePlatformAdmin(ctx, actor); err != nil {
		return api.Group{}, err
	}
	if groupID == "" {
		return api.Group{}, apperror.New("group id required")
	}
	if input.Name == nil || *input.Name == "" || len(*input.Name) > 100 {
		return api.Group{}, apperror.New("group name required, max 100 characters")
	}

	group, err := s.Repository.UpdateGroup(ctx, groupID, *input.Name)
	if err != nil {
		return api.Group{}, err
	}
	return api.Group{ID: group.ID, Name: group.Name}, nil
}

// DeleteGroup removes a group with its memberships and grants.
func (s *Service) DeleteGroup(ctx context.Context, actor auth.Principal, groupID string, now time.Time) error {
	if err := s.Access.RequirePlatformAdmin(ctx, actor); err != nil {
		return err
	}
	if groupID == "" {
		return apperror.New("group id required")
	}
	return s.Repository.DeleteGroup(ctx, groupID, now)
}

// AddGroupMember adds a user while allowing multiple groups.
func (s *Service) AddGroupMember(ctx context.Context, actor auth.Principal, groupID, userID string, now time.Time) (api.GroupMember, error) {
	if err := s.Access.RequirePlatformAdmin(ctx, actor); err != nil {
		return api.GroupMember{}, err
	}
	if groupID == "" || userID == "" {
		return api.GroupMember{}, apperror.New("group id and user id required")
	}
	ug, err := s.Repository.AddGroupMember(ctx, groupID, userID, now)
	if err != nil {
		return api.GroupMember{}, err
	}
	return api.GroupMember{GroupID: ug.GroupID, UserID: ug.UserID}, nil
}

// RemoveGroupMember removes only the specified group membership.
func (s *Service) RemoveGroupMember(ctx context.Context, actor auth.Principal, groupID, userID string, now time.Time) error {
	if err := s.Access.RequirePlatformAdmin(ctx, actor); err != nil {
		return err
	}
	if groupID == "" || userID == "" {
		return apperror.New("group id and user id required")
	}
	return s.Repository.RemoveGroupMember(ctx, groupID, userID, now)
}

// ListManagedDatabases lists all registered targets for administration.
func (s *Service) ListManagedDatabases(ctx context.Context, actor auth.Principal) ([]api.ManagedDatabaseSummary, error) {
	if err := s.Access.RequirePlatformAdmin(ctx, actor); err != nil {
		return nil, err
	}
	dbs, err := s.Repository.ListDatabases(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]api.ManagedDatabaseSummary, len(dbs))
	for i, db := range dbs {
		result[i] = api.ManagedDatabaseSummary{
			ID:           db.ID,
			Label:        db.Label,
			DatabaseName: db.DatabaseName,
			IsEnabled:    db.IsEnabled,
		}
	}
	return result, nil
}

// CreateManagedDatabase registers a PostgreSQL target.
func (s *Service) CreateManagedDatabase(ctx context.Context, actor auth.Principal, input api.CreateDatabaseRequest) (api.ManagedDatabaseSummary, error) {
	if err := s.Access.RequirePlatformAdmin(ctx, actor); err != nil {
		return api.ManagedDatabaseSummary{}, err
	}
	if input.Label == "" || input.Host == "" || input.DatabaseName == "" || input.SSLMode == "" {
		return api.ManagedDatabaseSummary{}, apperror.New("label, host, database_name, ssl_mode required")
	}
	if input.Port <= 0 {
		return api.ManagedDatabaseSummary{}, apperror.New("port must be positive")
	}
	if input.ReaderUsername == "" || input.ReaderPassword == "" || input.MigratorUsername == "" || input.MigratorPassword == "" {
		return api.ManagedDatabaseSummary{}, apperror.New("both reader and migrator credentials required")
	}

	readerEnc, err := s.Cipher.Encrypt(input.ReaderPassword)
	if err != nil {
		return api.ManagedDatabaseSummary{}, err
	}
	migratorEnc, err := s.Cipher.Encrypt(input.MigratorPassword)
	if err != nil {
		return api.ManagedDatabaseSummary{}, err
	}

	db := models.ManagedDatabase{
		Label:                 input.Label,
		Host:                  input.Host,
		Port:                  input.Port,
		DatabaseName:          input.DatabaseName,
		SSLMode:               input.SSLMode,
		ReaderUsername:        input.ReaderUsername,
		ReaderPasswordEnc:     readerEnc,
		MigratorUsername:      input.MigratorUsername,
		MigratorPasswordEnc:   migratorEnc,
		IsEnabled:             true,
	}

	created, err := s.Repository.CreateDatabase(ctx, db)
	if err != nil {
		return api.ManagedDatabaseSummary{}, err
	}

	return api.ManagedDatabaseSummary{
		ID:           created.ID,
		Label:        created.Label,
		DatabaseName: created.DatabaseName,
		IsEnabled:    created.IsEnabled,
	}, nil
}

// GetManagedDatabase returns editable connection settings without passwords.
func (s *Service) GetManagedDatabase(ctx context.Context, actor auth.Principal, databaseID string) (api.ManagedDatabaseDetail, error) {
	if err := s.Access.RequirePlatformAdmin(ctx, actor); err != nil {
		return api.ManagedDatabaseDetail{}, err
	}
	if databaseID == "" {
		return api.ManagedDatabaseDetail{}, apperror.New("database id required")
	}
	db, err := s.Repository.GetDatabase(ctx, databaseID)
	if err != nil {
		return api.ManagedDatabaseDetail{}, err
	}
	return api.ManagedDatabaseDetail{
		ID:              db.ID,
		Label:           db.Label,
		Host:            db.Host,
		Port:            db.Port,
		DatabaseName:    db.DatabaseName,
		SSLMode:         db.SSLMode,
		ReaderUsername:  db.ReaderUsername,
		MigratorUsername: db.MigratorUsername,
		IsEnabled:       db.IsEnabled,
	}, nil
}

// UpdateManagedDatabase updates only supplied connection fields.
func (s *Service) UpdateManagedDatabase(ctx context.Context, actor auth.Principal, databaseID string, input api.UpdateDatabaseRequest) (api.ManagedDatabaseSummary, error) {
	if err := s.Access.RequirePlatformAdmin(ctx, actor); err != nil {
		return api.ManagedDatabaseSummary{}, err
	}
	if databaseID == "" {
		return api.ManagedDatabaseSummary{}, apperror.New("database id required")
	}

	current, err := s.Repository.GetDatabase(ctx, databaseID)
	if err != nil {
		return api.ManagedDatabaseSummary{}, err
	}

	if input.Label != nil {
		current.Label = *input.Label
	}
	if input.Host != nil {
		current.Host = *input.Host
	}
	if input.Port != nil {
		if *input.Port <= 0 {
			return api.ManagedDatabaseSummary{}, apperror.New("port must be positive")
		}
		current.Port = *input.Port
	}
	if input.DatabaseName != nil {
		current.DatabaseName = *input.DatabaseName
	}
	if input.SSLMode != nil {
		current.SSLMode = *input.SSLMode
	}
	if input.ReaderUsername != nil {
		current.ReaderUsername = *input.ReaderUsername
	}
	if input.ReaderPassword != nil {
		enc, err := s.Cipher.Encrypt(*input.ReaderPassword)
		if err != nil {
			return api.ManagedDatabaseSummary{}, err
		}
		current.ReaderPasswordEnc = enc
	}
	if input.MigratorUsername != nil {
		current.MigratorUsername = *input.MigratorUsername
	}
	if input.MigratorPassword != nil {
		enc, err := s.Cipher.Encrypt(*input.MigratorPassword)
		if err != nil {
			return api.ManagedDatabaseSummary{}, err
		}
		current.MigratorPasswordEnc = enc
	}
	if input.IsEnabled != nil {
		current.IsEnabled = *input.IsEnabled
	}

	updated, err := s.Repository.UpdateDatabase(ctx, current)
	if err != nil {
		return api.ManagedDatabaseSummary{}, err
	}

	return api.ManagedDatabaseSummary{
		ID:           updated.ID,
		Label:        updated.Label,
		DatabaseName: updated.DatabaseName,
		IsEnabled:    updated.IsEnabled,
	}, nil
}

// DisableManagedDatabase soft deletes a registration while keeping history.
func (s *Service) DisableManagedDatabase(ctx context.Context, actor auth.Principal, databaseID string) error {
	if err := s.Access.RequirePlatformAdmin(ctx, actor); err != nil {
		return err
	}
	if databaseID == "" {
		return apperror.New("database id required")
	}
	return s.Repository.DisableDatabase(ctx, databaseID)
}

// TestDatabaseConnection checks a registered target connection.
func (s *Service) TestDatabaseConnection(ctx context.Context, actor auth.Principal, databaseID string) (api.TestConnectionResult, error) {
	if err := s.Access.RequirePlatformAdmin(ctx, actor); err != nil {
		return api.TestConnectionResult{}, err
	}
	if databaseID == "" {
		return api.TestConnectionResult{}, apperror.New("database id required")
	}
	err := s.Connections.TestConnection(ctx, databaseID)
	connected := err == nil
	return api.TestConnectionResult{Connected: connected}, nil
}

// SetGroupDatabaseAccess sets a group permission for one database.
func (s *Service) SetGroupDatabaseAccess(ctx context.Context, actor auth.Principal, groupID, databaseID string, input api.SetGroupAccessRequest, now time.Time) (api.GroupDatabaseAccess, error) {
	if err := s.Access.RequirePlatformAdmin(ctx, actor); err != nil {
		return api.GroupDatabaseAccess{}, err
	}
	if groupID == "" || databaseID == "" {
		return api.GroupDatabaseAccess{}, apperror.New("group id and database id required")
	}
	if input.AccessLevel != api.AccessViewOnly && input.AccessLevel != api.AccessViewEdit {
		return api.GroupDatabaseAccess{}, apperror.New("access level must be VIEW_ONLY or VIEW_EDIT")
	}

	grant := models.GroupAccess{
		GroupID:     groupID,
		DatabaseID:  databaseID,
		AccessLevel: models.AccessLevel(input.AccessLevel),
	}

	saved, err := s.Repository.SetGroupDatabaseAccess(ctx, grant, now)
	if err != nil {
		return api.GroupDatabaseAccess{}, err
	}

	return api.GroupDatabaseAccess{
		GroupID:     saved.GroupID,
		DatabaseID:  saved.DatabaseID,
		AccessLevel: api.AccessLevel(saved.AccessLevel),
	}, nil
}

// DeleteGroupDatabaseAccess removes a group permission for one database.
func (s *Service) DeleteGroupDatabaseAccess(ctx context.Context, actor auth.Principal, groupID, databaseID string, now time.Time) error {
	if err := s.Access.RequirePlatformAdmin(ctx, actor); err != nil {
		return err
	}
	if groupID == "" || databaseID == "" {
		return apperror.New("group id and database id required")
	}
	return s.Repository.DeleteGroupDatabaseAccess(ctx, groupID, databaseID, now)
}
