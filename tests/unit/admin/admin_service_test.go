package admin_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/admin"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/auth"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/models"
)

var errUnavailable = errors.New("repository unavailable")

// newService wires a fully functional Service with the supplied repo and,
// by default, an authorizing access checker and no-op cipher/connections.
func newService(repo admin.Repository) *admin.Service {
	return &admin.Service{
		Repository:   repo,
		Access:       &mockAccess{},
		Cipher:       &mockCipher{},
		Connections:  &mockConnections{},
		HashPassword: hashStub,
	}
}

// denyAccess returns a service whose access checker rejects every request.
func denyAccess(repo admin.Repository) *admin.Service {
	svc := newService(repo)
	svc.Access = &mockAccess{err: auth.ErrUnauthenticated}
	return svc
}

var adminPrincipal = makePrincipal("user-1", true)

func TestListUsers(t *testing.T) {
	ctx := context.Background()

	t.Run("requires platform admin", func(t *testing.T) {
		svc := denyAccess(&mockRepository{})
		_, err := svc.ListUsers(ctx, adminPrincipal)
		if !errors.Is(err, auth.ErrUnauthenticated) {
			t.Fatalf("error = %v, want %v", err, auth.ErrUnauthenticated)
		}
	})

	t.Run("repository error propagated", func(t *testing.T) {
		repo := &mockRepository{listUsersFn: func(context.Context) ([]models.User, error) {
			return nil, errUnavailable
		}}
		svc := newService(repo)
		_, err := svc.ListUsers(ctx, adminPrincipal)
		if !errors.Is(err, errUnavailable) {
			t.Fatalf("error = %v, want %v", err, errUnavailable)
		}
	})

	t.Run("maps users to admin response", func(t *testing.T) {
		repo := &mockRepository{listUsersFn: func(context.Context) ([]models.User, error) {
			return []models.User{
				{ID: "u1", Username: "alice", IsPlatformAdmin: true, IsActive: true},
				{ID: "u2", Username: "bob", IsPlatformAdmin: false, IsActive: false},
			}, nil
		}}
		svc := newService(repo)
		users, err := svc.ListUsers(ctx, adminPrincipal)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(users) != 2 {
			t.Fatalf("len(users) = %d, want 2", len(users))
		}
		if users[0].ID != "u1" || !users[0].IsPlatformAdmin || !users[0].IsActive {
			t.Fatalf("users[0] = %+v", users[0])
		}
		if users[1].Username != "bob" || users[1].IsPlatformAdmin || users[1].IsActive {
			t.Fatalf("users[1] = %+v", users[1])
		}
	})
}

func TestCreateUser(t *testing.T) {
	ctx := context.Background()

	t.Run("requires platform admin", func(t *testing.T) {
		svc := denyAccess(&mockRepository{})
		_, err := svc.CreateUser(ctx, adminPrincipal, api.CreateUserRequest{Username: "x", Password: "password123"})
		if !errors.Is(err, auth.ErrUnauthenticated) {
			t.Fatalf("error = %v, want %v", err, auth.ErrUnauthenticated)
		}
	})

	t.Run("rejects empty username or password", func(t *testing.T) {
		svc := newService(&mockRepository{})
		for _, input := range []api.CreateUserRequest{
			{Username: "", Password: "password123"},
			{Username: "alice", Password: ""},
		} {
			if _, err := svc.CreateUser(ctx, adminPrincipal, input); err == nil {
				t.Fatalf("input %+v accepted, want error", input)
			}
		}
	})

	t.Run("rejects short password", func(t *testing.T) {
		svc := newService(&mockRepository{})
		_, err := svc.CreateUser(ctx, adminPrincipal, api.CreateUserRequest{Username: "alice", Password: "short"})
		if err == nil {
			t.Fatal("short password accepted, want error")
		}
	})

	t.Run("hashes password and persists user", func(t *testing.T) {
		var saved models.User
		repo := &mockRepository{createUserFn: func(_ context.Context, u models.User) (models.User, error) {
			saved = u
			u.ID = "new-id"
			return u, nil
		}}
		svc := newService(repo)
		created, err := svc.CreateUser(ctx, adminPrincipal, api.CreateUserRequest{
			Username: "alice", Password: "password123", IsPlatformAdmin: true,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if created.ID != "new-id" || created.Username != "alice" || !created.IsPlatformAdmin || !created.IsActive {
			t.Fatalf("created = %+v", created)
		}
		if saved.PasswordHash != "hash:password123" {
			t.Fatalf("password hash = %q, want %q", saved.PasswordHash, "hash:password123")
		}
	})

	t.Run("repository error propagated", func(t *testing.T) {
		repo := &mockRepository{createUserFn: func(context.Context, models.User) (models.User, error) {
			return models.User{}, errUnavailable
		}}
		svc := newService(repo)
		_, err := svc.CreateUser(ctx, adminPrincipal, api.CreateUserRequest{Username: "alice", Password: "password123"})
		if !errors.Is(err, errUnavailable) {
			t.Fatalf("error = %v, want %v", err, errUnavailable)
		}
	})
}

func TestUpdateUser(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	t.Run("requires platform admin", func(t *testing.T) {
		svc := denyAccess(&mockRepository{})
		_, err := svc.UpdateUser(ctx, adminPrincipal, "u1", api.UpdateUserRequest{}, now)
		if !errors.Is(err, auth.ErrUnauthenticated) {
			t.Fatalf("error = %v, want %v", err, auth.ErrUnauthenticated)
		}
	})

	t.Run("rejects empty user id", func(t *testing.T) {
		svc := newService(&mockRepository{})
		active := true
		_, err := svc.UpdateUser(ctx, adminPrincipal, "", api.UpdateUserRequest{IsActive: &active}, now)
		if err == nil {
			t.Fatal("empty user id accepted, want error")
		}
	})

	t.Run("rejects no fields to update", func(t *testing.T) {
		svc := newService(&mockRepository{})
		_, err := svc.UpdateUser(ctx, adminPrincipal, "u1", api.UpdateUserRequest{}, now)
		if err == nil {
			t.Fatal("empty update accepted, want error")
		}
	})

	t.Run("rejects short new password", func(t *testing.T) {
		repo := &mockRepository{getUserFn: func(context.Context, string) (models.User, error) {
			return models.User{ID: "u1", Username: "alice"}, nil
		}}
		svc := newService(repo)
		short := "short"
		_, err := svc.UpdateUser(ctx, adminPrincipal, "u1", api.UpdateUserRequest{Password: &short}, now)
		if err == nil {
			t.Fatal("short password accepted, want error")
		}
	})

	t.Run("revokes sessions on password change", func(t *testing.T) {
		var revoke bool
		var saved models.User
		repo := &mockRepository{
			getUserFn: func(context.Context, string) (models.User, error) {
				return models.User{ID: "u1", Username: "alice"}, nil
			},
			updateUserFn: func(_ context.Context, u models.User, revokeSessions bool, _ time.Time) (models.User, error) {
				revoke = revokeSessions
				saved = u
				return u, nil
			},
		}
		svc := newService(repo)
		newPass := "newpassword123"
		updated, err := svc.UpdateUser(ctx, adminPrincipal, "u1", api.UpdateUserRequest{Password: &newPass}, now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !revoke {
			t.Fatal("password change did not revoke sessions")
		}
		if saved.PasswordHash != "hash:newpassword123" {
			t.Fatalf("password hash = %q", saved.PasswordHash)
		}
		if updated.Username != "alice" {
			t.Fatalf("updated = %+v", updated)
		}
	})

	t.Run("revokes sessions on deactivation", func(t *testing.T) {
		var revoke bool
		repo := &mockRepository{
			getUserFn: func(context.Context, string) (models.User, error) {
				return models.User{ID: "u1", Username: "alice", IsActive: true}, nil
			},
			updateUserFn: func(_ context.Context, u models.User, revokeSessions bool, _ time.Time) (models.User, error) {
				revoke = revokeSessions
				return u, nil
			},
		}
		svc := newService(repo)
		inactive := false
		_, err := svc.UpdateUser(ctx, adminPrincipal, "u1", api.UpdateUserRequest{IsActive: &inactive}, now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !revoke {
			t.Fatal("deactivation did not revoke sessions")
		}
	})

	t.Run("username only does not revoke", func(t *testing.T) {
		var revoke bool
		repo := &mockRepository{
			getUserFn: func(context.Context, string) (models.User, error) {
				return models.User{ID: "u1", Username: "alice"}, nil
			},
			updateUserFn: func(_ context.Context, u models.User, revokeSessions bool, _ time.Time) (models.User, error) {
				revoke = revokeSessions
				return u, nil
			},
		}
		svc := newService(repo)
		name := "alice2"
		_, err := svc.UpdateUser(ctx, adminPrincipal, "u1", api.UpdateUserRequest{Username: &name}, now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if revoke {
			t.Fatal("username change revoked sessions, want none")
		}
	})

	t.Run("revokes sessions on platform admin change", func(t *testing.T) {
		var revoke bool
		var saved models.User
		repo := &mockRepository{
			getUserFn: func(context.Context, string) (models.User, error) {
				return models.User{ID: "u1", Username: "alice", IsPlatformAdmin: false}, nil
			},
			updateUserFn: func(_ context.Context, u models.User, revokeSessions bool, _ time.Time) (models.User, error) {
				revoke = revokeSessions
				saved = u
				return u, nil
			},
		}
		svc := newService(repo)
		isAdmin := true
		updated, err := svc.UpdateUser(ctx, adminPrincipal, "u1", api.UpdateUserRequest{IsPlatformAdmin: &isAdmin}, now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !revoke {
			t.Fatal("platform admin change did not revoke sessions")
		}
		if !saved.IsPlatformAdmin || !updated.IsPlatformAdmin {
			t.Fatalf("saved=%+v updated=%+v, want IsPlatformAdmin=true", saved, updated)
		}
	})

	t.Run("repository error propagated", func(t *testing.T) {
		repo := &mockRepository{getUserFn: func(context.Context, string) (models.User, error) {
			return models.User{}, errUnavailable
		}}
		svc := newService(repo)
		name := "alice2"
		_, err := svc.UpdateUser(ctx, adminPrincipal, "u1", api.UpdateUserRequest{Username: &name}, now)
		if !errors.Is(err, errUnavailable) {
			t.Fatalf("error = %v, want %v", err, errUnavailable)
		}
	})
}

func TestGroups(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	t.Run("list requires admin and maps groups", func(t *testing.T) {
		svc := denyAccess(&mockRepository{})
		if _, err := svc.ListGroups(ctx, adminPrincipal); !errors.Is(err, auth.ErrUnauthenticated) {
			t.Fatalf("error = %v, want %v", err, auth.ErrUnauthenticated)
		}

		repo := &mockRepository{listGroupsFn: func(context.Context) ([]models.Group, error) {
			return []models.Group{{ID: "g1", Name: "devs"}, {ID: "g2", Name: "ops"}}, nil
		}}
		svc = newService(repo)
		groups, err := svc.ListGroups(ctx, adminPrincipal)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(groups) != 2 || groups[0].ID != "g1" || groups[1].Name != "ops" {
			t.Fatalf("groups = %+v", groups)
		}
	})

	t.Run("create rejects invalid names", func(t *testing.T) {
		svc := newService(&mockRepository{})
		for _, name := range []string{"", string(make([]byte, 101))} {
			if _, err := svc.CreateGroup(ctx, adminPrincipal, api.CreateGroupRequest{Name: name}); err == nil {
				t.Fatalf("name len %d accepted, want error", len(name))
			}
		}
	})

	t.Run("create returns created group", func(t *testing.T) {
		repo := &mockRepository{createGroupFn: func(_ context.Context, name string) (models.Group, error) {
			return models.Group{ID: "g1", Name: name}, nil
		}}
		svc := newService(repo)
		group, err := svc.CreateGroup(ctx, adminPrincipal, api.CreateGroupRequest{Name: "devs"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if group.ID != "g1" || group.Name != "devs" {
			t.Fatalf("group = %+v", group)
		}
	})

	t.Run("update rejects empty id or invalid name", func(t *testing.T) {
		svc := newService(&mockRepository{})
		name := "devs"
		if _, err := svc.UpdateGroup(ctx, adminPrincipal, "", api.UpdateGroupRequest{Name: &name}); err == nil {
			t.Fatal("empty group id accepted, want error")
		}
		if _, err := svc.UpdateGroup(ctx, adminPrincipal, "g1", api.UpdateGroupRequest{}); err == nil {
			t.Fatal("nil name accepted, want error")
		}
	})

	t.Run("delete rejects empty id", func(t *testing.T) {
		svc := newService(&mockRepository{})
		if err := svc.DeleteGroup(ctx, adminPrincipal, "", now); err == nil {
			t.Fatal("empty group id accepted, want error")
		}
	})

	t.Run("delete propagates repository error", func(t *testing.T) {
		repo := &mockRepository{deleteGroupFn: func(context.Context, string, time.Time) error {
			return errUnavailable
		}}
		svc := newService(repo)
		if err := svc.DeleteGroup(ctx, adminPrincipal, "g1", now); !errors.Is(err, errUnavailable) {
			t.Fatalf("error = %v, want %v", err, errUnavailable)
		}
	})

	t.Run("add member rejects empty ids and maps result", func(t *testing.T) {
		svc := newService(&mockRepository{})
		if _, err := svc.AddGroupMember(ctx, adminPrincipal, "", "u1", now); err == nil {
			t.Fatal("empty group id accepted, want error")
		}
		if _, err := svc.AddGroupMember(ctx, adminPrincipal, "g1", "", now); err == nil {
			t.Fatal("empty user id accepted, want error")
		}

		repo := &mockRepository{addGroupMemberFn: func(_ context.Context, g, u string, _ time.Time) (models.UserGroup, error) {
			return models.UserGroup{GroupID: g, UserID: u}, nil
		}}
		svc = newService(repo)
		member, err := svc.AddGroupMember(ctx, adminPrincipal, "g1", "u1", now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if member.GroupID != "g1" || member.UserID != "u1" {
			t.Fatalf("member = %+v", member)
		}
	})

	t.Run("remove member rejects empty ids", func(t *testing.T) {
		svc := newService(&mockRepository{})
		if err := svc.RemoveGroupMember(ctx, adminPrincipal, "", "u1", now); err == nil {
			t.Fatal("empty group id accepted, want error")
		}
		if err := svc.RemoveGroupMember(ctx, adminPrincipal, "g1", "", now); err == nil {
			t.Fatal("empty user id accepted, want error")
		}
	})
}

func TestListManagedDatabases(t *testing.T) {
	ctx := context.Background()

	t.Run("requires platform admin", func(t *testing.T) {
		svc := denyAccess(&mockRepository{})
		_, err := svc.ListManagedDatabases(ctx, adminPrincipal)
		if !errors.Is(err, auth.ErrUnauthenticated) {
			t.Fatalf("error = %v, want %v", err, auth.ErrUnauthenticated)
		}
	})

	t.Run("maps databases to summary", func(t *testing.T) {
		repo := &mockRepository{listDatabasesFn: func(context.Context) ([]models.ManagedDatabase, error) {
			return []models.ManagedDatabase{
				{ID: "db1", Label: "One", DatabaseName: "db1", IsEnabled: true},
				{ID: "db2", Label: "Two", DatabaseName: "db2", IsEnabled: false},
			}, nil
		}}
		svc := newService(repo)
		dbs, err := svc.ListManagedDatabases(ctx, adminPrincipal)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(dbs) != 2 || dbs[0].Label != "One" || dbs[1].IsEnabled {
			t.Fatalf("dbs = %+v", dbs)
		}
	})
}

func TestCreateManagedDatabase(t *testing.T) {
	ctx := context.Background()

	valid := api.CreateDatabaseRequest{
		Label:            "prod",
		Host:             "localhost",
		Port:             5432,
		DatabaseName:     "app",
		SSLMode:          "require",
		ReaderUsername:   "reader",
		ReaderPassword:   "reader-pass",
		MigratorUsername: "migrator",
		MigratorPassword: "migrator-pass",
	}

	t.Run("requires platform admin", func(t *testing.T) {
		svc := denyAccess(&mockRepository{})
		_, err := svc.CreateManagedDatabase(ctx, adminPrincipal, valid)
		if !errors.Is(err, auth.ErrUnauthenticated) {
			t.Fatalf("error = %v, want %v", err, auth.ErrUnauthenticated)
		}
	})

	t.Run("rejects empty label", func(t *testing.T) {
		svc := &admin.Service{
			Repository: &mockRepository{},
			Access:     &mockAccess{},
			Cipher:     &mockCipher{},
		}

		input := api.CreateDatabaseRequest{
			Label: "   ", // whitespace only
		}

		_, err := svc.CreateManagedDatabase(ctx, adminPrincipal, input)
		if err == nil {
			t.Fatal("expected error for missing label, got nil")
		}
	})

	
	t.Run("applies default values when optional fields are omitted", func(t *testing.T) {
		var capturedDB models.ManagedDatabase

		repo := &mockRepository{
			createDatabaseFn: func(ctx context.Context, db models.ManagedDatabase) (models.ManagedDatabase, error) {
				capturedDB = db
				db.ID = "db-123"
				return db, nil
			},
		}

		svc := &admin.Service{
			Repository: repo,
			Access:     &mockAccess{},
			Cipher:     &mockCipher{},
		}

		// Provide only the required connection label
		input := api.CreateDatabaseRequest{
			Label: "Production Replica",
		}

		summary, err := svc.CreateManagedDatabase(ctx, adminPrincipal, input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Assert output summary
		if summary.ID != "db-123" {
			t.Errorf("ID = %q, want %q", summary.ID, "db-123")
		}
		if summary.Label != "Production Replica" {
			t.Errorf("Label = %q, want %q", summary.Label, "Production Replica")
		}

		// Assert that default values were populated before saving to the repository
		if capturedDB.Host != "postgres" {
			t.Errorf("Host = %q, want default %q", capturedDB.Host, "postgres")
		}
		if capturedDB.Port != 5432 {
			t.Errorf("Port = %d, want default %d", capturedDB.Port, 5432)
		}
		if capturedDB.DatabaseName != "target_db" {
			t.Errorf("DatabaseName = %q, want default %q", capturedDB.DatabaseName, "target_db")
		}
		if capturedDB.SSLMode != "disable" {
			t.Errorf("SSLMode = %q, want default %q", capturedDB.SSLMode, "disable")
		}
		if capturedDB.ReaderUsername != "admin" {
			t.Errorf("ReaderUsername = %q, want default %q", capturedDB.ReaderUsername, "admin")
		}
		if capturedDB.ReaderPasswordEnc != "enc:root" {
			t.Errorf("ReaderPasswordEnc = %q, want encrypted default %q", capturedDB.ReaderPasswordEnc, "enc:root")
		}
		if capturedDB.MigratorUsername != "admin" {
			t.Errorf("MigratorUsername = %q, want default %q", capturedDB.MigratorUsername, "admin")
		}
		if capturedDB.MigratorPasswordEnc != "enc:root" {
			t.Errorf("MigratorPasswordEnc = %q, want encrypted default %q", capturedDB.MigratorPasswordEnc, "enc:root")
		}
	})

	t.Run("encrypts credentials and persists", func(t *testing.T) {
		var saved models.ManagedDatabase
		repo := &mockRepository{createDatabaseFn: func(_ context.Context, db models.ManagedDatabase) (models.ManagedDatabase, error) {
			saved = db
			db.ID = "db-id"
			return db, nil
		}}
		svc := newService(repo)
		created, err := svc.CreateManagedDatabase(ctx, adminPrincipal, valid)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if created.ID != "db-id" || created.Label != "prod" || !created.IsEnabled {
			t.Fatalf("created = %+v", created)
		}
		if saved.ReaderPasswordEnc != "enc:reader-pass" || saved.MigratorPasswordEnc != "enc:migrator-pass" {
			t.Fatalf("credentials not encrypted: %+v", saved)
		}
	})

	t.Run("cipher error propagated", func(t *testing.T) {
		svc := newService(&mockRepository{})
		svc.Cipher = &mockCipher{encryptFn: func(string) (string, error) { return "", errUnavailable }}
		_, err := svc.CreateManagedDatabase(ctx, adminPrincipal, valid)
		if !errors.Is(err, errUnavailable) {
			t.Fatalf("error = %v, want %v", err, errUnavailable)
		}
	})
}

func TestGetManagedDatabase(t *testing.T) {
	ctx := context.Background()

	t.Run("requires admin and rejects empty id", func(t *testing.T) {
		svc := denyAccess(&mockRepository{})
		if _, err := svc.GetManagedDatabase(ctx, adminPrincipal, "db1"); !errors.Is(err, auth.ErrUnauthenticated) {
			t.Fatalf("error = %v, want %v", err, auth.ErrUnauthenticated)
		}
		svc = newService(&mockRepository{})
		if _, err := svc.GetManagedDatabase(ctx, adminPrincipal, ""); err == nil {
			t.Fatal("empty id accepted, want error")
		}
	})

	t.Run("maps database to detail without secrets", func(t *testing.T) {
		repo := &mockRepository{getDatabaseFn: func(context.Context, string) (models.ManagedDatabase, error) {
			return models.ManagedDatabase{
				ID: "db1", Label: "prod", Host: "h", Port: 5432, DatabaseName: "app",
				SSLMode: "require", ReaderUsername: "r", MigratorUsername: "m",
				ReaderPasswordEnc: "SECRET", MigratorPasswordEnc: "SECRET", IsEnabled: true,
			}, nil
		}}
		svc := newService(repo)
		db, err := svc.GetManagedDatabase(ctx, adminPrincipal, "db1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if db.ID != "db1" || db.Port != 5432 || db.ReaderUsername != "r" {
			t.Fatalf("db = %+v", db)
		}
	})
}

func TestUpdateManagedDatabase(t *testing.T) {
	ctx := context.Background()

	t.Run("rejects empty id", func(t *testing.T) {
		svc := newService(&mockRepository{})
		_, err := svc.UpdateManagedDatabase(ctx, adminPrincipal, "", api.UpdateDatabaseRequest{})
		if err == nil {
			t.Fatal("empty id accepted, want error")
		}
	})

	t.Run("rejects non-positive port", func(t *testing.T) {
		repo := &mockRepository{getDatabaseFn: func(context.Context, string) (models.ManagedDatabase, error) {
			return models.ManagedDatabase{ID: "db1"}, nil
		}}
		svc := newService(repo)
		port := 0
		_, err := svc.UpdateManagedDatabase(ctx, adminPrincipal, "db1", api.UpdateDatabaseRequest{Port: &port})
		if err == nil {
			t.Fatal("zero port accepted, want error")
		}
	})

	t.Run("encrypts new reader password", func(t *testing.T) {
		var saved models.ManagedDatabase
		repo := &mockRepository{
			getDatabaseFn: func(context.Context, string) (models.ManagedDatabase, error) {
				return models.ManagedDatabase{ID: "db1", Label: "prod"}, nil
			},
			updateDatabaseFn: func(_ context.Context, db models.ManagedDatabase) (models.ManagedDatabase, error) {
				saved = db
				return db, nil
			},
		}
		svc := newService(repo)
		pass := "new-reader"
		_, err := svc.UpdateManagedDatabase(ctx, adminPrincipal, "db1", api.UpdateDatabaseRequest{ReaderPassword: &pass})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if saved.ReaderPasswordEnc != "enc:new-reader" {
			t.Fatalf("reader password enc = %q", saved.ReaderPasswordEnc)
		}
	})
}

func TestDisableManagedDatabase(t *testing.T) {
	ctx := context.Background()

	t.Run("requires admin and rejects empty id", func(t *testing.T) {
		svc := denyAccess(&mockRepository{})
		if err := svc.DisableManagedDatabase(ctx, adminPrincipal, "db1"); !errors.Is(err, auth.ErrUnauthenticated) {
			t.Fatalf("error = %v, want %v", err, auth.ErrUnauthenticated)
		}
		svc = newService(&mockRepository{})
		if err := svc.DisableManagedDatabase(ctx, adminPrincipal, ""); err == nil {
			t.Fatal("empty id accepted, want error")
		}
	})

	t.Run("propagates repository error", func(t *testing.T) {
		repo := &mockRepository{disableDatabaseFn: func(context.Context, string) error { return errUnavailable }}
		svc := newService(repo)
		if err := svc.DisableManagedDatabase(ctx, adminPrincipal, "db1"); !errors.Is(err, errUnavailable) {
			t.Fatalf("error = %v, want %v", err, errUnavailable)
		}
	})
}

func TestTestDatabaseConnection(t *testing.T) {
	ctx := context.Background()

	t.Run("rejects empty id", func(t *testing.T) {
		svc := newService(&mockRepository{})
		if _, err := svc.TestDatabaseConnection(ctx, adminPrincipal, ""); err == nil {
			t.Fatal("empty id accepted, want error")
		}
	})

	t.Run("reports connected on success", func(t *testing.T) {
		svc := newService(&mockRepository{})
		svc.Connections = &mockConnections{}
		result, err := svc.TestDatabaseConnection(ctx, adminPrincipal, "db1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Connected {
			t.Fatal("connected = false, want true")
		}
	})

	t.Run("reports disconnected without error on failure", func(t *testing.T) {
		svc := newService(&mockRepository{})
		svc.Connections = &mockConnections{err: errUnavailable}
		result, err := svc.TestDatabaseConnection(ctx, adminPrincipal, "db1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Connected {
			t.Fatal("connected = true, want false")
		}
	})
}

func TestGroupDatabaseAccess(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	t.Run("set rejects empty ids and invalid level", func(t *testing.T) {
		svc := newService(&mockRepository{})
		if _, err := svc.SetGroupDatabaseAccess(ctx, adminPrincipal, "", "db1", api.SetGroupAccessRequest{AccessLevel: api.AccessViewOnly}, now); err == nil {
			t.Fatal("empty group id accepted, want error")
		}
		if _, err := svc.SetGroupDatabaseAccess(ctx, adminPrincipal, "g1", "db1", api.SetGroupAccessRequest{AccessLevel: "WRITE"}, now); err == nil {
			t.Fatal("invalid level accepted, want error")
		}
	})

	t.Run("set maps saved grant", func(t *testing.T) {
		repo := &mockRepository{setGroupDatabaseAccessFn: func(_ context.Context, grant models.GroupAccess, _ time.Time) (models.GroupAccess, error) {
			return grant, nil
		}}
		svc := newService(repo)
		grant, err := svc.SetGroupDatabaseAccess(ctx, adminPrincipal, "g1", "db1", api.SetGroupAccessRequest{AccessLevel: api.AccessViewEdit}, now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if grant.GroupID != "g1" || grant.DatabaseID != "db1" || grant.AccessLevel != api.AccessViewEdit {
			t.Fatalf("grant = %+v", grant)
		}
	})

	t.Run("delete rejects empty ids", func(t *testing.T) {
		svc := newService(&mockRepository{})
		if err := svc.DeleteGroupDatabaseAccess(ctx, adminPrincipal, "", "db1", now); err == nil {
			t.Fatal("empty group id accepted, want error")
		}
		if err := svc.DeleteGroupDatabaseAccess(ctx, adminPrincipal, "g1", "", now); err == nil {
			t.Fatal("empty database id accepted, want error")
		}
	})

	t.Run("delete propagates repository error", func(t *testing.T) {
		repo := &mockRepository{deleteGroupAccessFn: func(context.Context, string, string, time.Time) error {
			return errUnavailable
		}}
		svc := newService(repo)
		if err := svc.DeleteGroupDatabaseAccess(ctx, adminPrincipal, "g1", "db1", now); !errors.Is(err, errUnavailable) {
			t.Fatalf("error = %v, want %v", err, errUnavailable)
		}
	})
}
