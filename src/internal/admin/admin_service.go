package admin

import (
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/apperror"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/auth"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/models"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/store"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/telemetry"
)

type Repository interface {
	ListAccessSnapshot(context.Context) (api.AccessSnapshot, error)
	ListUsers(ctx context.Context) ([]models.User, error)
	GetUser(ctx context.Context, userID string) (models.User, error)
	CreateUser(ctx context.Context, user models.User) (models.User, error)
	UpdateUser(ctx context.Context, user models.User, revokeSessions bool, now time.Time) (models.User, error)
	ListGroups(ctx context.Context) ([]models.Group, error)
	CreateGroup(ctx context.Context, name, leaderID string) (models.Group, error)
	UpdateGroup(ctx context.Context, groupID string, name, leaderID *string) (models.Group, error)
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

type DisabledDatabaseDeleter interface {
	DeleteDisabledDatabase(ctx context.Context, databaseID string) error
}

type AdminChecker interface {
	RequirePlatformAdmin(context.Context, auth.Principal) error
}

type ConnectionTester interface {
	TestConnection(context.Context, string) error
}

type poolInvalidator interface {
	InvalidateTargetPools(string)
}

// Service maps persistence models to the existing API response types.
type Service struct {
	Repository   Repository
	Access       AdminChecker
	Cipher       store.CredentialCipher
	Connections  ConnectionTester
	HashPassword func(string) (string, error)
}

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

func (s *Service) CreateUser(ctx context.Context, actor auth.Principal, input api.CreateUserRequest) (api.AdminUser, error) {
	if err := s.Access.RequirePlatformAdmin(ctx, actor); err != nil {
		return api.AdminUser{}, err
	}
	if input.Username == "" || input.Password == "" {
		return api.AdminUser{}, fmt.Errorf("%w: username and password required", apperror.ErrInvalidInput)
	}
	if len(input.Password) < 8 {
		return api.AdminUser{}, fmt.Errorf("%w: password must be at least 8 characters", apperror.ErrInvalidInput)
	}

	hash, err := s.HashPassword(input.Password)
	if err != nil {
		return api.AdminUser{}, err
	}

	user := models.User{
		Username:        input.Username,
		PasswordHash:    hash,
		IsPlatformAdmin: input.IsPlatformAdmin,
		IsActive:        true,
	}

	created, err := s.Repository.CreateUser(ctx, user)
	if err != nil {
		return api.AdminUser{}, err
	}

	telemetry.Log(ctx, slog.LevelInfo, "admin.user_created", "actor", actor.User.ID, "target_user_id", created.ID, "username", created.Username, "is_platform_admin", created.IsPlatformAdmin, "status", "success")

	return api.AdminUser{
		ID:              created.ID,
		Username:        created.Username,
		IsPlatformAdmin: created.IsPlatformAdmin,
		IsActive:        created.IsActive,
	}, nil
}

func (s *Service) UpdateUser(ctx context.Context, actor auth.Principal, userID string, input api.UpdateUserRequest, now time.Time) (api.AdminUser, error) {
	if err := s.Access.RequirePlatformAdmin(ctx, actor); err != nil {
		return api.AdminUser{}, err
	}
	if userID == "" {
		return api.AdminUser{}, fmt.Errorf("%w: user id required", apperror.ErrInvalidInput)
	}
	if input.Username == nil && input.Password == nil && input.IsActive == nil && input.IsPlatformAdmin == nil {
		return api.AdminUser{}, fmt.Errorf("%w: no fields to update", apperror.ErrInvalidInput)
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
			return api.AdminUser{}, fmt.Errorf("%w: password must be at least 8 characters", apperror.ErrInvalidInput)
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
	if input.IsPlatformAdmin != nil {
		if current.IsPlatformAdmin != *input.IsPlatformAdmin {
			revokeSessions = true
		}
		current.IsPlatformAdmin = *input.IsPlatformAdmin
	}

	updated, err := s.Repository.UpdateUser(ctx, current, revokeSessions, now)
	if err != nil {
		return api.AdminUser{}, err
	}

	telemetry.Log(ctx, slog.LevelInfo, "admin.user_updated", "actor", actor.User.ID, "target_user_id", updated.ID, "is_active", updated.IsActive, "is_platform_admin", updated.IsPlatformAdmin, "sessions_revoked", revokeSessions, "status", "success")

	return api.AdminUser{
		ID:              updated.ID,
		Username:        updated.Username,
		IsPlatformAdmin: updated.IsPlatformAdmin,
		IsActive:        updated.IsActive,
	}, nil
}

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
		result[i] = groupResponse(g)
	}
	return result, nil
}

func (s *Service) CreateGroup(ctx context.Context, actor auth.Principal, input api.CreateGroupRequest) (api.Group, error) {
	if err := s.Access.RequirePlatformAdmin(ctx, actor); err != nil {
		return api.Group{}, err
	}
	if input.Name == "" || len(input.Name) > 100 {
		return api.Group{}, fmt.Errorf("%w: group name required, max 100 characters", apperror.ErrInvalidInput)
	}
	if !validLeaderID(input.LeaderID) {
		return api.Group{}, fmt.Errorf("%w: leader_id must be a valid user UUID", apperror.ErrInvalidInput)
	}

	group, err := s.Repository.CreateGroup(ctx, input.Name, input.LeaderID)
	if err != nil {
		return api.Group{}, err
	}
	res := groupResponse(group)
	telemetry.Log(ctx, slog.LevelInfo, "admin.group_created", "actor", actor.User.ID, "group_id", res.ID, "group_name", res.Name, "leader_id", res.LeaderID, "status", "success")
	return res, nil
}

func (s *Service) UpdateGroup(ctx context.Context, actor auth.Principal, groupID string, input api.UpdateGroupRequest) (api.Group, error) {
	if err := s.Access.RequirePlatformAdmin(ctx, actor); err != nil {
		return api.Group{}, err
	}
	if groupID == "" {
		return api.Group{}, fmt.Errorf("%w: group id required", apperror.ErrInvalidInput)
	}
	if input.Name == nil && input.LeaderID == nil {
		return api.Group{}, fmt.Errorf("%w: no fields to update", apperror.ErrInvalidInput)
	}
	if input.Name != nil && (*input.Name == "" || len(*input.Name) > 100) {
		return api.Group{}, fmt.Errorf("%w: group name required, max 100 characters", apperror.ErrInvalidInput)
	}
	if input.LeaderID != nil && !validLeaderID(*input.LeaderID) {
		return api.Group{}, fmt.Errorf("%w: leader_id must be a valid user UUID", apperror.ErrInvalidInput)
	}

	group, err := s.Repository.UpdateGroup(ctx, groupID, input.Name, input.LeaderID)
	if err != nil {
		return api.Group{}, err
	}
	res := groupResponse(group)
	telemetry.Log(ctx, slog.LevelInfo, "admin.group_updated", "actor", actor.User.ID, "group_id", res.ID, "group_name", res.Name, "leader_id", res.LeaderID, "status", "success")
	return res, nil
}

func validLeaderID(id string) bool {
	if len(id) != 36 || id[8] != '-' || id[13] != '-' || id[18] != '-' || id[23] != '-' {
		return false
	}
	if id == "00000000-0000-0000-0000-000000000000" {
		return false
	}
	decoded, err := hex.DecodeString(strings.ReplaceAll(id, "-", ""))
	return err == nil && len(decoded) == 16
}

func groupResponse(group models.Group) api.Group {
	result := api.Group{ID: group.ID, Name: group.Name}
	if group.LeaderID != nil {
		result.LeaderID = *group.LeaderID
	}
	return result
}

func (s *Service) DeleteGroup(ctx context.Context, actor auth.Principal, groupID string, now time.Time) error {
	if err := s.Access.RequirePlatformAdmin(ctx, actor); err != nil {
		return err
	}
	if groupID == "" {
		return fmt.Errorf("%w: group id required", apperror.ErrInvalidInput)
	}
	if err := s.Repository.DeleteGroup(ctx, groupID, now); err != nil {
		return err
	}
	telemetry.Log(ctx, slog.LevelInfo, "admin.group_deleted", "actor", actor.User.ID, "group_id", groupID, "status", "success")
	return nil
}

func (s *Service) AddGroupMember(ctx context.Context, actor auth.Principal, groupID, userID string, now time.Time) (api.GroupMember, error) {
	if err := s.Access.RequirePlatformAdmin(ctx, actor); err != nil {
		return api.GroupMember{}, err
	}
	if groupID == "" || userID == "" {
		return api.GroupMember{}, fmt.Errorf("%w: group id and user id required", apperror.ErrInvalidInput)
	}
	ug, err := s.Repository.AddGroupMember(ctx, groupID, userID, now)
	if err != nil {
		return api.GroupMember{}, err
	}
	telemetry.Log(ctx, slog.LevelInfo, "admin.group_member_added", "actor", actor.User.ID, "group_id", ug.GroupID, "target_user_id", ug.UserID, "status", "success")
	return api.GroupMember{GroupID: ug.GroupID, UserID: ug.UserID}, nil
}

func (s *Service) RemoveGroupMember(ctx context.Context, actor auth.Principal, groupID, userID string, now time.Time) error {
	if err := s.Access.RequirePlatformAdmin(ctx, actor); err != nil {
		return err
	}
	if groupID == "" || userID == "" {
		return fmt.Errorf("%w: group id and user id required", apperror.ErrInvalidInput)
	}
	if err := s.Repository.RemoveGroupMember(ctx, groupID, userID, now); err != nil {
		return err
	}
	telemetry.Log(ctx, slog.LevelInfo, "admin.group_member_removed", "actor", actor.User.ID, "group_id", groupID, "target_user_id", userID, "status", "success")
	return nil
}

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
			Engine:       db.TargetEngine(),
			IsEnabled:    db.IsEnabled,
		}
	}
	return result, nil
}

func (s *Service) CreateManagedDatabase(ctx context.Context, actor auth.Principal, input api.CreateDatabaseRequest) (api.ManagedDatabaseSummary, error) {
	if err := s.Access.RequirePlatformAdmin(ctx, actor); err != nil {
		return api.ManagedDatabaseSummary{}, err
	}

	input.Label = strings.TrimSpace(input.Label)
	if input.Label == "" {
		return api.ManagedDatabaseSummary{}, fmt.Errorf("%w: connection label is required", apperror.ErrInvalidInput)
	}

	engine, err := normalizeEngine(input.Engine)
	if err != nil {
		return api.ManagedDatabaseSummary{}, err
	}
	input.Engine = engine
	if strings.TrimSpace(input.Host) == "" {
		input.Host = "postgres"
		if engine == models.EngineMySQL {
			input.Host = "mysql"
		}
	}
	if input.Port <= 0 {
		input.Port = 5432
		if engine == models.EngineMySQL {
			input.Port = 3306
		}
	}
	if strings.TrimSpace(input.DatabaseName) == "" {
		input.DatabaseName = "target_db"
	}
	if strings.TrimSpace(input.SSLMode) == "" {
		input.SSLMode = "disable"
	}
	if err := validateSSLMode(engine, input.SSLMode); err != nil {
		return api.ManagedDatabaseSummary{}, err
	}
	if strings.TrimSpace(input.ReaderUsername) == "" {
		input.ReaderUsername = "admin"
	}
	if input.ReaderPassword == "" {
		input.ReaderPassword = "root"
	}
	if strings.TrimSpace(input.MigratorUsername) == "" {
		input.MigratorUsername = input.ReaderUsername
	}
	if input.MigratorPassword == "" {
		input.MigratorPassword = input.ReaderPassword
	}

	if input.UseReaderCredentials {
		input.MigratorUsername = input.ReaderUsername
		input.MigratorPassword = input.ReaderPassword
	}

	readerEnc, err := s.Cipher.Encrypt(input.ReaderPassword)
	if err != nil {
		return api.ManagedDatabaseSummary{}, fmt.Errorf("%w: encryption failed: %w", apperror.ErrInternal, err)
	}
	migratorEnc, err := s.Cipher.Encrypt(input.MigratorPassword)
	if err != nil {
		return api.ManagedDatabaseSummary{}, fmt.Errorf("%w: encryption failed: %w", apperror.ErrInternal, err)
	}

	db := models.ManagedDatabase{
		Engine:              input.Engine,
		Label:               input.Label,
		Host:                input.Host,
		Port:                input.Port,
		DatabaseName:        input.DatabaseName,
		SSLMode:             input.SSLMode,
		ReaderUsername:      input.ReaderUsername,
		ReaderPasswordEnc:   readerEnc,
		MigratorUsername:    input.MigratorUsername,
		MigratorPasswordEnc: migratorEnc,
		IsEnabled:           true,
	}

	created, err := s.Repository.CreateDatabase(ctx, db)
	if err != nil {
		return api.ManagedDatabaseSummary{}, err
	}

	telemetry.Log(ctx, slog.LevelInfo, "admin.database_created", "actor", actor.User.ID, "database_id", created.ID, "label", created.Label, "engine", created.TargetEngine(), "status", "success")

	return api.ManagedDatabaseSummary{
		ID:           created.ID,
		Label:        created.Label,
		DatabaseName: created.DatabaseName,
		Engine:       created.TargetEngine(),
		IsEnabled:    created.IsEnabled,
	}, nil
}

func (s *Service) GetManagedDatabase(ctx context.Context, actor auth.Principal, databaseID string) (api.ManagedDatabaseDetail, error) {
	if err := s.Access.RequirePlatformAdmin(ctx, actor); err != nil {
		return api.ManagedDatabaseDetail{}, err
	}
	if databaseID == "" {
		return api.ManagedDatabaseDetail{}, fmt.Errorf("%w: database id required", apperror.ErrInvalidInput)
	}
	db, err := s.Repository.GetDatabase(ctx, databaseID)
	if err != nil {
		return api.ManagedDatabaseDetail{}, err
	}
	return api.ManagedDatabaseDetail{
		ID:               db.ID,
		Label:            db.Label,
		Host:             db.Host,
		Port:             db.Port,
		DatabaseName:     db.DatabaseName,
		Engine:           db.TargetEngine(),
		SSLMode:          db.SSLMode,
		ReaderUsername:   db.ReaderUsername,
		MigratorUsername: db.MigratorUsername,
		IsEnabled:        db.IsEnabled,
	}, nil
}

func (s *Service) UpdateManagedDatabase(ctx context.Context, actor auth.Principal, databaseID string, input api.UpdateDatabaseRequest) (api.ManagedDatabaseSummary, error) {
	if err := s.Access.RequirePlatformAdmin(ctx, actor); err != nil {
		return api.ManagedDatabaseSummary{}, err
	}
	if databaseID == "" {
		return api.ManagedDatabaseSummary{}, fmt.Errorf("%w: database id required", apperror.ErrInvalidInput)
	}

	current, err := s.Repository.GetDatabase(ctx, databaseID)
	if err != nil {
		return api.ManagedDatabaseSummary{}, err
	}

	current, err = s.applyConnectionChanges(current, input)
	if err != nil {
		return api.ManagedDatabaseSummary{}, err
	}

	updated, err := s.Repository.UpdateDatabase(ctx, current)
	if err != nil {
		return api.ManagedDatabaseSummary{}, err
	}

	if inv, ok := s.Connections.(poolInvalidator); ok {
		inv.InvalidateTargetPools(databaseID)
	}

	telemetry.Log(ctx, slog.LevelInfo, "admin.database_updated", "actor", actor.User.ID, "database_id", updated.ID, "label", updated.Label, "is_enabled", updated.IsEnabled, "status", "success")

	return api.ManagedDatabaseSummary{
		ID:           updated.ID,
		Label:        updated.Label,
		DatabaseName: updated.DatabaseName,
		Engine:       updated.TargetEngine(),
		IsEnabled:    updated.IsEnabled,
	}, nil
}

func (s *Service) DisableManagedDatabase(ctx context.Context, actor auth.Principal, databaseID string) error {
	if err := s.Access.RequirePlatformAdmin(ctx, actor); err != nil {
		return err
	}
	if databaseID == "" {
		return fmt.Errorf("%w: database id required", apperror.ErrInvalidInput)
	}
	if err := s.Repository.DisableDatabase(ctx, databaseID); err != nil {
		return err
	}
	if inv, ok := s.Connections.(poolInvalidator); ok {
		inv.InvalidateTargetPools(databaseID)
	}
	telemetry.Log(ctx, slog.LevelInfo, "admin.database_disabled", "actor", actor.User.ID, "database_id", databaseID, "status", "success")
	return nil
}

func (s *Service) DeleteManagedDatabase(ctx context.Context, actor auth.Principal, databaseID string) error {
	if err := s.Access.RequirePlatformAdmin(ctx, actor); err != nil {
		return err
	}
	if databaseID == "" {
		return apperror.New("database id required")
	}
	deleter, ok := s.Repository.(DisabledDatabaseDeleter)
	if !ok {
		return apperror.ErrInternal
	}
	if err := deleter.DeleteDisabledDatabase(ctx, databaseID); err != nil {
		return err
	}
	if inv, ok := s.Connections.(poolInvalidator); ok {
		inv.InvalidateTargetPools(databaseID)
	}
	telemetry.Log(ctx, slog.LevelInfo, "admin.database_deleted", "actor", actor.User.ID, "database_id", databaseID, "status", "success")
	return nil
}

func (s *Service) TestDatabaseConnection(ctx context.Context, actor auth.Principal, databaseID string) (api.TestConnectionResult, error) {
	if err := s.Access.RequirePlatformAdmin(ctx, actor); err != nil {
		return api.TestConnectionResult{}, err
	}
	if databaseID == "" {
		return api.TestConnectionResult{}, fmt.Errorf("%w: database id required", apperror.ErrInvalidInput)
	}
	err := s.Connections.TestConnection(ctx, databaseID)
	if err != nil {
		return api.TestConnectionResult{Connected: false, Message: err.Error()}, nil
	}
	return api.TestConnectionResult{Connected: true, Message: "Reader and migrator connections succeeded."}, nil
}

func (s *Service) SetGroupDatabaseAccess(ctx context.Context, actor auth.Principal, groupID, databaseID string, input api.SetGroupAccessRequest, now time.Time) (api.GroupDatabaseAccess, error) {
	if err := s.Access.RequirePlatformAdmin(ctx, actor); err != nil {
		return api.GroupDatabaseAccess{}, err
	}
	if groupID == "" || databaseID == "" {
		return api.GroupDatabaseAccess{}, fmt.Errorf("%w: group id and database id required", apperror.ErrInvalidInput)
	}
	if input.AccessLevel != api.AccessViewOnly && input.AccessLevel != api.AccessViewEdit {
		return api.GroupDatabaseAccess{}, fmt.Errorf("%w: access level must be VIEW_ONLY or VIEW_EDIT", apperror.ErrInvalidInput)
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

	telemetry.Log(ctx, slog.LevelInfo, "admin.access_granted", "actor", actor.User.ID, "group_id", saved.GroupID, "database_id", saved.DatabaseID, "access_level", string(saved.AccessLevel), "status", "success")

	return api.GroupDatabaseAccess{
		GroupID:     saved.GroupID,
		DatabaseID:  saved.DatabaseID,
		AccessLevel: api.AccessLevel(saved.AccessLevel),
	}, nil
}

func (s *Service) DeleteGroupDatabaseAccess(ctx context.Context, actor auth.Principal, groupID, databaseID string, now time.Time) error {
	if err := s.Access.RequirePlatformAdmin(ctx, actor); err != nil {
		return err
	}
	if groupID == "" || databaseID == "" {
		return fmt.Errorf("%w: group id and database id required", apperror.ErrInvalidInput)
	}
	if err := s.Repository.DeleteGroupDatabaseAccess(ctx, groupID, databaseID, now); err != nil {
		return err
	}
	telemetry.Log(ctx, slog.LevelInfo, "admin.access_revoked", "actor", actor.User.ID, "group_id", groupID, "database_id", databaseID, "status", "success")
	return nil
}

func (s *Service) applyConnectionChanges(current models.ManagedDatabase, input api.UpdateDatabaseRequest) (models.ManagedDatabase, error) {
	if input.Engine != nil {
		engine, err := normalizeEngine(*input.Engine)
		if err != nil {
			return current, err
		}
		current.Engine = engine
	}
	if input.Label != nil {
		current.Label = *input.Label
	}
	if input.Host != nil {
		current.Host = *input.Host
	}
	if input.Port != nil {
		if *input.Port <= 0 {
			return current, fmt.Errorf("%w: port must be positive", apperror.ErrInvalidInput)
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
			return current, err
		}
		current.ReaderPasswordEnc = enc
	}
	if input.MigratorUsername != nil {
		current.MigratorUsername = *input.MigratorUsername
	}
	if input.MigratorPassword != nil {
		enc, err := s.Cipher.Encrypt(*input.MigratorPassword)
		if err != nil {
			return current, err
		}
		current.MigratorPasswordEnc = enc
	}
	if input.IsEnabled != nil {
		current.IsEnabled = *input.IsEnabled
	}

	if input.UseReaderCredentials {
		current.MigratorUsername = current.ReaderUsername
		current.MigratorPasswordEnc = current.ReaderPasswordEnc
	}
	if current.SSLMode != "" {
		if err := validateSSLMode(current.TargetEngine(), current.SSLMode); err != nil {
			return current, err
		}
	}
	return current, nil
}

func normalizeEngine(engine string) (string, error) {
	engine = strings.ToLower(strings.TrimSpace(engine))
	if engine == "" {
		engine = models.EnginePostgres
	}
	if engine != models.EnginePostgres && engine != models.EngineMySQL {
		return "", fmt.Errorf("%w: engine must be postgres or mysql", apperror.ErrInvalidInput)
	}
	return engine, nil
}

func validateSSLMode(engine, mode string) error {
	if mode == "disable" || mode == "require" || mode == "verify-full" || (engine == models.EnginePostgres && mode == "verify-ca") {
		return nil
	}
	return fmt.Errorf("%w: unsupported SSL mode for %s", apperror.ErrInvalidInput, engine)
}
func (s *Service) ListAccessSnapshot(ctx context.Context, actor auth.Principal) (api.AccessSnapshot, error) {
	if err := s.Access.RequirePlatformAdmin(ctx, actor); err != nil {
		return api.AccessSnapshot{}, err
	}
	return s.Repository.ListAccessSnapshot(ctx)
}

func (s *Service) TestConnectionDraft(ctx context.Context, actor auth.Principal, input api.TestConnectionRequest) (api.TestConnectionResult, error) {
	if err := s.Access.RequirePlatformAdmin(ctx, actor); err != nil {
		return api.TestConnectionResult{}, err
	}
	var current models.ManagedDatabase
	var err error
	if input.DatabaseID != "" {
		current, err = s.Repository.GetDatabase(ctx, input.DatabaseID)
		if err != nil {
			return api.TestConnectionResult{}, err
		}
	}
	current, err = s.applyConnectionChanges(current, input.UpdateDatabaseRequest)
	if err != nil {
		return api.TestConnectionResult{}, err
	}
	if strings.TrimSpace(current.Host) == "" || current.Port < 1 || current.Port > 65535 || strings.TrimSpace(current.DatabaseName) == "" || strings.TrimSpace(current.ReaderUsername) == "" || strings.TrimSpace(current.MigratorUsername) == "" || current.ReaderPasswordEnc == "" || current.MigratorPasswordEnc == "" {
		return api.TestConnectionResult{}, fmt.Errorf("%w: complete the connection details and both credentials before testing", apperror.ErrInvalidInput)
	}
	switch current.SSLMode {
	case "disable", "require", "verify-ca", "verify-full":
	default:
		return api.TestConnectionResult{}, fmt.Errorf("%w: invalid SSL mode", apperror.ErrInvalidInput)
	}
	tester, ok := s.Connections.(interface {
		TestConfiguration(context.Context, models.ManagedDatabase) error
	})
	if !ok {
		return api.TestConnectionResult{}, apperror.ErrNotImplemented
	}
	if err := tester.TestConfiguration(ctx, current); err != nil {
		return api.TestConnectionResult{Connected: false, Message: err.Error()}, nil
	}
	return api.TestConnectionResult{Connected: true, Message: "Reader and migrator connections succeeded."}, nil
}
