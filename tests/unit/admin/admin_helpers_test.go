package admin_test

import (
	"context"
	"time"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/admin"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/auth"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/models"
)

// mockRepository embeds admin.Repository and overrides only the methods a test
// exercises. Unoverridden methods panic loudly (same pattern as auth tests).
type mockRepository struct {
	admin.Repository

	listUsersFn              func(context.Context) ([]models.User, error)
	getUserFn                func(context.Context, string) (models.User, error)
	createUserFn             func(context.Context, models.User) (models.User, error)
	updateUserFn             func(context.Context, models.User, bool, time.Time) (models.User, error)
	listGroupsFn             func(context.Context) ([]models.Group, error)
	createGroupFn            func(context.Context, string) (models.Group, error)
	deleteGroupFn            func(context.Context, string, time.Time) error
	addGroupMemberFn         func(context.Context, string, string, time.Time) (models.UserGroup, error)
	listDatabasesFn          func(context.Context) ([]models.ManagedDatabase, error)
	getDatabaseFn            func(context.Context, string) (models.ManagedDatabase, error)
	createDatabaseFn         func(context.Context, models.ManagedDatabase) (models.ManagedDatabase, error)
	updateDatabaseFn         func(context.Context, models.ManagedDatabase) (models.ManagedDatabase, error)
	disableDatabaseFn        func(context.Context, string) error
	setGroupDatabaseAccessFn func(context.Context, models.GroupAccess, time.Time) (models.GroupAccess, error)
	deleteGroupAccessFn      func(context.Context, string, string, time.Time) error
}

func (m *mockRepository) ListUsers(ctx context.Context) ([]models.User, error) {
	return m.listUsersFn(ctx)
}

func (m *mockRepository) GetUser(ctx context.Context, userID string) (models.User, error) {
	return m.getUserFn(ctx, userID)
}

func (m *mockRepository) CreateUser(ctx context.Context, user models.User) (models.User, error) {
	return m.createUserFn(ctx, user)
}

func (m *mockRepository) UpdateUser(ctx context.Context, user models.User, revokeSessions bool, now time.Time) (models.User, error) {
	return m.updateUserFn(ctx, user, revokeSessions, now)
}

func (m *mockRepository) ListGroups(ctx context.Context) ([]models.Group, error) {
	return m.listGroupsFn(ctx)
}

func (m *mockRepository) CreateGroup(ctx context.Context, name string) (models.Group, error) {
	return m.createGroupFn(ctx, name)
}

func (m *mockRepository) DeleteGroup(ctx context.Context, groupID string, now time.Time) error {
	return m.deleteGroupFn(ctx, groupID, now)
}

func (m *mockRepository) AddGroupMember(ctx context.Context, groupID, userID string, now time.Time) (models.UserGroup, error) {
	return m.addGroupMemberFn(ctx, groupID, userID, now)
}

func (m *mockRepository) ListDatabases(ctx context.Context) ([]models.ManagedDatabase, error) {
	return m.listDatabasesFn(ctx)
}

func (m *mockRepository) GetDatabase(ctx context.Context, databaseID string) (models.ManagedDatabase, error) {
	return m.getDatabaseFn(ctx, databaseID)
}

func (m *mockRepository) CreateDatabase(ctx context.Context, database models.ManagedDatabase) (models.ManagedDatabase, error) {
	return m.createDatabaseFn(ctx, database)
}

func (m *mockRepository) UpdateDatabase(ctx context.Context, database models.ManagedDatabase) (models.ManagedDatabase, error) {
	return m.updateDatabaseFn(ctx, database)
}

func (m *mockRepository) DisableDatabase(ctx context.Context, databaseID string) error {
	return m.disableDatabaseFn(ctx, databaseID)
}

func (m *mockRepository) SetGroupDatabaseAccess(ctx context.Context, grant models.GroupAccess, now time.Time) (models.GroupAccess, error) {
	return m.setGroupDatabaseAccessFn(ctx, grant, now)
}

func (m *mockRepository) DeleteGroupDatabaseAccess(ctx context.Context, groupID, databaseID string, now time.Time) error {
	return m.deleteGroupAccessFn(ctx, groupID, databaseID, now)
}

// mockAccess implements admin.AdminChecker.
type mockAccess struct {
	err error
}

func (m *mockAccess) RequirePlatformAdmin(context.Context, auth.Principal) error {
	return m.err
}

// mockCipher implements store.CredentialCipher.
type mockCipher struct {
	encryptFn func(string) (string, error)
	decryptFn func(string) (string, error)
}

func (m *mockCipher) Encrypt(plaintext string) (string, error) {
	if m.encryptFn != nil {
		return m.encryptFn(plaintext)
	}
	return "enc:" + plaintext, nil
}

func (m *mockCipher) Decrypt(ciphertext string) (string, error) {
	if m.decryptFn != nil {
		return m.decryptFn(ciphertext)
	}
	return "", nil
}

// mockConnections implements admin.ConnectionTester.
type mockConnections struct {
	err error
}

func (m *mockConnections) TestConnection(context.Context, string) error {
	return m.err
}

// makePrincipal builds a platform admin principal for authorization checks.
func makePrincipal(userID string, isAdmin bool) auth.Principal {
	return auth.Principal{
		User: api.LoginResponse{
			ID:              userID,
			Username:        "admin",
			IsPlatformAdmin: isAdmin,
		},
		SessionID: "session-123",
	}
}

// hashStub is a deterministic password hashing stand-in.
func hashStub(password string) (string, error) {
	return "hash:" + password, nil
}
