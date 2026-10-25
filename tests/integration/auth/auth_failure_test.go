package auth_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/app"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/auth"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/models"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/tests/internal/testutil"
)

type failingRepository struct{ auth.Repository }

func (failingRepository) FindSessionByTokenHash(context.Context, string) (models.Session, error) {
	return models.Session{}, errors.New("database unavailable")
}

func TestSessionMiddlewarePreservesRepositoryErrors(t *testing.T) {
	router, err := app.NewRouter(failingRepository{}, testutil.AuthConfig())
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/api/v1/auth/me", nil)
	r.AddCookie(&http.Cookie{Name: "sid", Value: strings.Repeat("a", 43)})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500: %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "database unavailable") {
		t.Fatal("internal error leaked")
	}
}

func TestFindUserByIDPreservesDatabaseErrors(t *testing.T) {
	db := testutil.Database(t)
	if err := db.Exec("DROP TABLE users CASCADE").Error; err != nil {
		t.Fatal(err)
	}
	repo := &auth.PostgresRepository{DB: db}
	service := &auth.Service{Repository: repo}
	actor := auth.Principal{}
	actor.User.ID = "00000000-0000-0000-0000-000000000001"
	_, err := service.Me(context.Background(), actor)
	if err == nil {
		t.Fatal("missing database error")
	}
	// A database outage must not be mapped to an unknown/unauthenticated user.
	w := httptest.NewRecorder()
	auth.WriteAuthError(w, err)
	if w.Code != 500 {
		t.Fatalf("database error mapped to %d: %v", w.Code, err)
	}
}
