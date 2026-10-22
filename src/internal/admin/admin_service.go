package admin

import (
	"context"
	"time"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/apperror"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/auth"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/store"
)

type Repository interface {
	ListUsers(ctx context.Context) ([]api.AdminUser, error)
	GetUser(ctx context.Context, userID string) (auth.UserRecord, error)
	CreateUser(ctx context.Context, user auth.UserRecord) (api.AdminUser, error)
	UpdateUser(ctx context.Context, user auth.UserRecord, revokeSessions bool, now time.Time) (api.AdminUser, error)
	ListGroups(ctx context.Context) ([]api.Group, error)
	CreateGroup(ctx context.Context, name string) (api.Group, error)
	UpdateGroup(ctx context.Context, groupID, name string) (api.Group, error)
	DeleteGroup(ctx context.Context, groupID string, now time.Time) error
	AddGroupMember(ctx context.Context, groupID, userID string, now time.Time) (api.GroupMember, error)
	RemoveGroupMember(ctx context.Context, groupID, userID string, now time.Time) error
	ListDatabases(ctx context.Context) ([]api.ManagedDatabaseSummary, error)
	GetDatabase(ctx context.Context, databaseID string) (store.DatabaseRecord, error)
	CreateDatabase(ctx context.Context, database store.DatabaseRecord) (api.ManagedDatabaseSummary, error)
	UpdateDatabase(ctx context.Context, database store.DatabaseRecord) (api.ManagedDatabaseSummary, error)
	DisableDatabase(ctx context.Context, databaseID string) error
	SetGroupDatabaseAccess(ctx context.Context, grant api.GroupDatabaseAccess, now time.Time) (api.GroupDatabaseAccess, error)
	DeleteGroupDatabaseAccess(ctx context.Context, groupID, databaseID string, now time.Time) error
}

type AdminChecker interface {
	RequirePlatformAdmin(context.Context, auth.Principal) error
}

type ConnectionTester interface {
	TestConnection(context.Context, string) error
}

type Service struct {
	Repository   Repository
	Access       AdminChecker
	Cipher       store.CredentialCipher
	Connections  ConnectionTester
	HashPassword func(string) (string, error)
}

// ListUsers lists platform accounts.
func (s *Service) ListUsers(ctx context.Context, actor auth.Principal) ([]api.AdminUser, error) {
	// TODO Require platform admin access and ask the repository for safe user fields.
	return nil, apperror.ErrNotImplemented
}

// CreateUser creates an account with a password hash.
func (s *Service) CreateUser(ctx context.Context, actor auth.Principal, input api.CreateUserRequest) (api.AdminUser, error) {
	// TODO Require platform admin access and validate the username and password.
	// TODO Use HashPassword then persist a UserRecord with active state.
	// TODO Return api.AdminUser without the password hash.
	return api.AdminUser{}, apperror.ErrNotImplemented
}

// UpdateUser applies only supplied user fields.
func (s *Service) UpdateUser(ctx context.Context, actor auth.Principal, userID string, input api.UpdateUserRequest, now time.Time) (api.AdminUser, error) {
	// TODO Require platform admin access and reject an empty patch or invalid values.
	// TODO Load the current record and merge non nil fields and hash a changed password.
	// TODO Revoke sessions on deactivation or password change in the same metadata transaction.
	// TODO Prevent lost updates rather than silently overwriting concurrent patches.
	return api.AdminUser{}, apperror.ErrNotImplemented
}

// ListGroups lists platform groups.
func (s *Service) ListGroups(ctx context.Context, actor auth.Principal) ([]api.Group, error) {
	// TODO Require platform admin access then list groups.
	return nil, apperror.ErrNotImplemented
}

// CreateGroup creates a named group.
func (s *Service) CreateGroup(ctx context.Context, actor auth.Principal, input api.CreateGroupRequest) (api.Group, error) {
	// TODO Require platform admin access and validate a nonempty name of at most 100 characters.
	// TODO Persist the group and report duplicate names as conflicts.
	return api.Group{}, apperror.ErrNotImplemented
}

// UpdateGroup updates a group name.
func (s *Service) UpdateGroup(ctx context.Context, actor auth.Principal, groupID string, input api.UpdateGroupRequest) (api.Group, error) {
	// TODO Require platform admin access and require a valid non nil name before updating.
	return api.Group{}, apperror.ErrNotImplemented
}

// DeleteGroup removes a group with its memberships and grants.
func (s *Service) DeleteGroup(ctx context.Context, actor auth.Principal, groupID string, now time.Time) error {
	// TODO Require platform admin access then delete and revoke affected sessions atomically.
	return apperror.ErrNotImplemented
}

// AddGroupMember adds a user while allowing multiple groups.
func (s *Service) AddGroupMember(ctx context.Context, actor auth.Principal, groupID, userID string, now time.Time) (api.GroupMember, error) {
	// TODO Require platform admin access and verify the user and group exist.
	// TODO Add the membership and revoke affected sessions so login issues fresh tokens.
	return api.GroupMember{}, apperror.ErrNotImplemented
}

// RemoveGroupMember removes only the specified group membership.
func (s *Service) RemoveGroupMember(ctx context.Context, actor auth.Principal, groupID, userID string, now time.Time) error {
	// TODO Require platform admin access and remove the membership with session revocation.
	// TODO Make subsequent access checks read current grants rather than a permission cache.
	return apperror.ErrNotImplemented
}

// ListManagedDatabases lists all registered targets for administration.
func (s *Service) ListManagedDatabases(ctx context.Context, actor auth.Principal) ([]api.ManagedDatabaseSummary, error) {
	// TODO Require platform admin access and list summaries without credentials.
	return nil, apperror.ErrNotImplemented
}

// CreateManagedDatabase registers a PostgreSQL target.
func (s *Service) CreateManagedDatabase(ctx context.Context, actor auth.Principal, input api.CreateDatabaseRequest) (api.ManagedDatabaseSummary, error) {
	// TODO Require platform admin access and validate label and host and port and SSL mode.
	// TODO Constrain permitted destinations according to the deployment network policy.
	// TODO Encrypt both passwords with Cipher then persist a DatabaseRecord.
	// TODO Keep plaintext passwords and DSNs out of responses and logs.
	return api.ManagedDatabaseSummary{}, apperror.ErrNotImplemented
}

// GetManagedDatabase returns editable connection settings without passwords.
func (s *Service) GetManagedDatabase(ctx context.Context, actor auth.Principal, databaseID string) (api.ManagedDatabaseDetail, error) {
	// TODO Require platform admin access then return only DatabaseRecord.Detail.
	return api.ManagedDatabaseDetail{}, apperror.ErrNotImplemented
}

// UpdateManagedDatabase updates only supplied connection fields.
func (s *Service) UpdateManagedDatabase(ctx context.Context, actor auth.Principal, databaseID string, input api.UpdateDatabaseRequest) (api.ManagedDatabaseSummary, error) {
	// TODO Require platform admin access and load current settings and merge non nil fields.
	// TODO Validate merged settings and encrypt only passwords that were supplied.
	// TODO Keep existing ciphertext when a password field is absent.
	// TODO Persist safely against concurrent updates and invalidate old target pools.
	return api.ManagedDatabaseSummary{}, apperror.ErrNotImplemented
}

// DisableManagedDatabase soft deletes a registration while keeping history.
func (s *Service) DisableManagedDatabase(ctx context.Context, actor auth.Principal, databaseID string) error {
	// TODO Require platform admin access and set disabled rather than deleting metadata.
	// TODO Invalidate target pools so later operations cannot reuse a disabled connection.
	return apperror.ErrNotImplemented
}

// TestDatabaseConnection checks a registered target connection.
func (s *Service) TestDatabaseConnection(ctx context.Context, actor auth.Principal, databaseID string) (api.TestConnectionResult, error) {
	// TODO Require platform admin access then test reader and migrator with a timeout.
	// TODO Return sanitized failures and set Connected only after both checks pass.
	return api.TestConnectionResult{}, apperror.ErrNotImplemented
}

// SetGroupDatabaseAccess sets a group permission for one database.
func (s *Service) SetGroupDatabaseAccess(ctx context.Context, actor auth.Principal, groupID, databaseID string, input api.SetGroupAccessRequest, now time.Time) (api.GroupDatabaseAccess, error) {
	// TODO Require platform admin access and accept only VIEW_ONLY or VIEW_EDIT.
	// TODO Persist the grant and invalidate affected sessions in one metadata transaction.
	// TODO A fresh login after an increase must generate a new session identifier.
	return api.GroupDatabaseAccess{}, apperror.ErrNotImplemented
}

// DeleteGroupDatabaseAccess removes a group permission for one database.
func (s *Service) DeleteGroupDatabaseAccess(ctx context.Context, actor auth.Principal, groupID, databaseID string, now time.Time) error {
	// TODO Require platform admin access and revoke the pair and affected sessions atomically.
	// TODO Preserve other group grants and check effective permissions on every request.
	return apperror.ErrNotImplemented
}
