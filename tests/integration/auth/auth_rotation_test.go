package auth_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/app"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/auth"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/models"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/tests/internal/testutil"
)

// Both requests read the same session before either can rotate. Everything
// else, including the conditional UPDATE and retry, uses the real repository.
type simultaneousSessionReads struct {
	auth.Repository
	reads atomic.Int32
	ready chan struct{}
}

func (r *simultaneousSessionReads) FindSessionByTokenHash(ctx context.Context, hash string) (models.Session, error) {
	session, err := r.Repository.FindSessionByTokenHash(ctx, hash)
	if err != nil {
		return session, err
	}
	if count := r.reads.Add(1); count <= 2 {
		if count == 2 {
			close(r.ready)
		}
		select {
		case <-r.ready:
		case <-ctx.Done():
			return models.Session{}, ctx.Err()
		}
	}
	return session, nil
}

func TestConcurrentSessionRotation(t *testing.T) {
	db := testutil.Database(t)
	user := models.User{Username: "parallel-user", PasswordHash: "unused", IsActive: true}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	token := strings.Repeat("a", 43)
	session := models.Session{
		UserID: user.ID, TokenHash: fmt.Sprintf("%x", sha256.Sum256([]byte(token))),
		CSRFTokenHash: strings.Repeat("c", 64), CreatedAt: now.Add(-17 * time.Minute),
		LastSeenAt: now, RotatedAt: now.Add(-16 * time.Minute), AbsoluteExpiresAt: now.Add(time.Hour),
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatal(err)
	}
	repo := &simultaneousSessionReads{Repository: &auth.PostgresRepository{DB: db}, ready: make(chan struct{})}
	router, err := app.NewRouter(repo, testutil.AuthConfig())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	responses := make(chan *httptest.ResponseRecorder, 2)
	for range 2 {
		go func() {
			r := httptest.NewRequest("GET", "/api/v1/auth/me", nil).WithContext(ctx)
			r.AddCookie(&http.Cookie{Name: "sid", Value: token})
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			responses <- w
		}()
	}
	rotations := 0
	for range 2 {
		w := <-responses
		if w.Code != 200 {
			t.Errorf("concurrent auth = %d, want 200: %s", w.Code, w.Body.String())
		}
		for _, cookie := range w.Result().Cookies() {
			if cookie.Name == "sid" {
				rotations++
			}
		}
	}
	if rotations != 1 {
		t.Fatalf("session rotated %d times, want exactly once", rotations)
	}
}
