package auth_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/auth"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/config"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/models"
)

// Only the repository calls used by ValidateSession are needed in these tests.
type sessionRepository struct {
	auth.Repository
	session models.Session
	user    models.User
}

func (r *sessionRepository) FindSessionByTokenHash(context.Context, string) (models.Session, error) {
	return r.session, nil
}

func (r *sessionRepository) FindUserByID(context.Context, string) (models.User, error) {
	return r.user, nil
}

func (r *sessionRepository) TouchSession(context.Context, string, time.Time) error {
	return nil
}

func (r *sessionRepository) RotateSession(_ context.Context, _, expectedHash, nextHash string, now, graceUntil time.Time) error {
	if r.session.TokenHash != expectedHash {
		return errors.New("unexpected rotation conflict in test stub")
	}
	r.session.PreviousTokenHash = &expectedHash
	r.session.PreviousValidUntil = &graceUntil
	r.session.TokenHash = nextHash
	r.session.RotatedAt = now
	return nil
}

func TestValidateSessionTimeouts(t *testing.T) {
	now := time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)
	token := strings.Repeat("a", 43)
	for _, tc := range []struct {
		name     string
		lastSeen time.Time
		expires  time.Time
		revoked  *time.Time
		wantErr  bool
	}{
		{"active", now.Add(-time.Minute), now.Add(time.Hour), nil, false},
		{"idle boundary", now.Add(-30 * time.Minute), now.Add(time.Hour), nil, true},
		{"idle expired", now.Add(-31 * time.Minute), now.Add(time.Hour), nil, true},
		{"absolute expired", now, now, nil, true},
		{"revoked", now, now.Add(time.Hour), &now, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &sessionRepository{
				user: models.User{ID: "user", IsActive: true},
				session: models.Session{
					ID: "session", UserID: "user", TokenHash: tokenHash(token),
					LastSeenAt: tc.lastSeen, RotatedAt: now,
					AbsoluteExpiresAt: tc.expires, RevokedAt: tc.revoked,
				},
			}
			svc := &auth.Service{Repository: repo, Policy: config.SessionConfig{
				IdleTimeout: 30 * time.Minute, RotationInterval: 15 * time.Minute,
			}}
			got, err := svc.ValidateSession(context.Background(), token, now)
			if tc.wantErr {
				if !errors.Is(err, auth.ErrUnauthenticated) {
					t.Fatalf("got %v, want unauthenticated", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Principal.User.ID != "user" {
				t.Fatalf("wrong principal: %+v", got.Principal)
			}
		})
	}
}

func TestValidateSessionReturnsRotatedToken(t *testing.T) {
	now := time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)
	token := strings.Repeat("a", 43)
	expires := now.Add(time.Hour)
	repo := &sessionRepository{
		user: models.User{ID: "user", IsActive: true},
		session: models.Session{
			ID: "session", UserID: "user", TokenHash: tokenHash(token),
			LastSeenAt: now, RotatedAt: now.Add(-15 * time.Minute), AbsoluteExpiresAt: expires,
		},
	}
	svc := &auth.Service{Repository: repo, Policy: config.SessionConfig{
		IdleTimeout: 30 * time.Minute, RotationInterval: 15 * time.Minute, GracePeriod: 30 * time.Second,
	}}
	got, err := svc.ValidateSession(context.Background(), token, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.RotatedToken) != 43 || got.RotatedToken == token {
		t.Fatal("new token was not returned to the cookie writer")
	}
	if !got.Session.AbsoluteExpiresAt.Equal(expires) {
		t.Fatal("rotation extended absolute expiration")
	}
	if _, err := svc.ValidateSession(context.Background(), token, now.Add(29*time.Second)); err != nil {
		t.Fatalf("previous token within grace: %v", err)
	}
	if _, err := svc.ValidateSession(context.Background(), token, now.Add(30*time.Second)); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatalf("previous token after grace: %v", err)
	}
}

func tokenHash(token string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(token))) }
