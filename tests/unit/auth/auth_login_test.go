package auth_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/auth"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/models"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/tests/internal/testutil"
)

type loginRepository struct {
	auth.Repository
	user               models.User
	findErr, createErr error
	created            *models.Session
}

func (r *loginRepository) FindUserByUsername(context.Context, string) (models.User, error) {
	return r.user, r.findErr
}

func (r *loginRepository) CreateSession(_ context.Context, session *models.Session) error {
	copy := *session
	r.created = &copy
	return r.createErr
}

func TestLogin(t *testing.T) {
	hash, err := auth.HashPassword("test-password")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)
	unavailable := errors.New("repository unavailable")
	for _, tc := range []struct {
		name, username, password    string
		inactive                    bool
		findErr, createErr, wantErr error
	}{
		{name: "success", username: "tester", password: "test-password"},
		{name: "wrong password", username: "tester", password: "wrong", wantErr: auth.ErrInvalidCredentials},
		{name: "inactive account", username: "tester", password: "test-password", inactive: true, wantErr: auth.ErrInvalidCredentials},
		{name: "empty username", password: "test-password", wantErr: auth.ErrInvalidCredentials},
		{name: "long username", username: strings.Repeat("a", 65), password: "test-password", wantErr: auth.ErrInvalidCredentials},
		{name: "empty password", username: "tester", wantErr: auth.ErrInvalidCredentials},
		{name: "long password", username: "tester", password: strings.Repeat("a", 73), wantErr: auth.ErrInvalidCredentials},
		{name: "lookup failure", username: "tester", password: "test-password", findErr: unavailable, wantErr: unavailable},
		{name: "session persistence failure", username: "tester", password: "test-password", createErr: unavailable, wantErr: unavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &loginRepository{user: models.User{ID: "user", Username: "tester", PasswordHash: hash, IsActive: !tc.inactive}, findErr: tc.findErr, createErr: tc.createErr}
			svc := &auth.Service{Repository: repo, Policy: testutil.AuthConfig().Session}
			result, err := svc.Login(context.Background(), api.LoginRequest{Username: tc.username, Password: tc.password}, now)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr != nil {
				if tc.createErr == nil && repo.created != nil {
					t.Fatal("failed login created a session")
				}
				return
			}
			if result.User.ID != "user" || repo.created == nil {
				t.Fatal("missing user or session")
			}
			if len(result.SessionToken) != 43 || len(result.CSRFToken) != 43 || result.SessionToken == result.CSRFToken {
				t.Fatal("invalid independent tokens")
			}
			if repo.created.TokenHash != tokenHash(result.SessionToken) || repo.created.CSRFTokenHash != tokenHash(result.CSRFToken) {
				t.Fatal("session must store token hashes")
			}
			if !repo.created.AbsoluteExpiresAt.Equal(now.Add(24 * time.Hour)) {
				t.Fatal("wrong absolute expiration")
			}
		})
	}
}

func TestPasswordHashAndVerification(t *testing.T) {
	hash, err := auth.HashPassword("test-password")
	if err != nil {
		t.Fatal(err)
	}
	if hash == "test-password" {
		t.Fatal("password was not hashed")
	}
	if err := auth.VerifyPassword("test-password", hash); err != nil {
		t.Fatal(err)
	}
	if auth.VerifyPassword("wrong", hash) == nil {
		t.Fatal("wrong password accepted")
	}
	if auth.VerifyPassword("test-password", "invalid-hash") == nil {
		t.Fatal("invalid hash accepted")
	}
	if _, err := auth.HashPassword(strings.Repeat("a", 73)); err == nil {
		t.Fatal("oversized password accepted")
	}
}

func TestValidateCSRF(t *testing.T) {
	token := strings.Repeat("a", 43)
	session := models.Session{CSRFTokenHash: tokenHash(token)}
	svc := &auth.Service{}
	for _, input := range []string{token, "", "short", strings.Repeat("b", 43)} {
		err := svc.ValidateCSRF(context.Background(), session, input)
		if input == token && err != nil {
			t.Fatal(err)
		}
		if input != token && !errors.Is(err, auth.ErrCSRF) {
			t.Fatalf("invalid CSRF %q accepted", input)
		}
	}
}
