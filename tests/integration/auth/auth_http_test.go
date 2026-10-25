package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/app"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/auth"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/models"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/tests/internal/testutil"
)

func TestRouterRequiresSessionAndSameOrigin(t *testing.T) {
	router, err := app.NewRouter(nil, testutil.AuthConfig())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		method, path string
		want         int
	}{
		{"GET", "/health", 200},
		{"GET", "/login", 200},
		{"GET", "/databases", 200},
		{"GET", "/static/app.css", 200},
		{"GET", "/api/v1/auth/me", 401},
		{"POST", "/api/v1/auth/logout", 401},
		{"POST", "/api/v1/auth/login", 403},
	} {
		r := httptest.NewRequest(tc.method, tc.path, nil)
		r.Header.Set("Origin", "https://attacker.test")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Errorf("%s %s = %d, want %d", tc.method, tc.path, w.Code, tc.want)
		}
	}
}

func TestAuthHTTPIntegration(t *testing.T) {
	ctx := context.Background()
	db := testutil.Database(t)
	hash, err := auth.HashPassword("test-password")
	if err != nil {
		t.Fatal(err)
	}
	user := models.User{Username: "tester", PasswordHash: hash, IsActive: true}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	repo := &auth.PostgresRepository{DB: db}
	cfg := testutil.AuthConfig()
	router, err := app.NewRouter(repo, cfg)
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path, body, origin, csrf string, cookies []*http.Cookie) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		if csrf != "" {
			r.Header.Set("X-CSRF-Token", csrf)
		}
		for _, cookie := range cookies {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	status := func(w *httptest.ResponseRecorder, want int) {
		t.Helper()
		if w.Code != want {
			t.Fatalf("status = %d, want %d: %s", w.Code, want, w.Body.String())
		}
	}
	body := `{"username":"tester","password":"test-password"}`
	origin := cfg.PublicOrigin
	status(request("POST", "/api/v1/auth/login", `{}`, origin, "", nil), 401)
	status(request("POST", "/api/v1/auth/login", `{"username":"missing","password":"test-password"}`, origin, "", nil), 401)
	status(request("POST", "/api/v1/auth/login", `{"username":"tester","password":"wrong"}`, origin, "", nil), 401)
	status(request("POST", "/api/v1/auth/login", body+` {}`, origin, "", nil), 400)
	login := request("POST", "/api/v1/auth/login", body, origin, "", nil)
	status(login, 200)
	cookies := login.Result().Cookies()
	if len(cookies) != 2 {
		t.Fatalf("expected 2 cookies, got %d", len(cookies))
	}
	var csrf string
	for _, cookie := range cookies {
		if cookie.Path != "/" || cookie.SameSite != http.SameSiteLaxMode || cookie.Expires.IsZero() {
			t.Fatalf("invalid cookie flags: %+v", cookie)
		}
		if cookie.Name == "sid" && !cookie.HttpOnly {
			t.Fatal("session cookie must be HttpOnly")
		}
		if cookie.Name == "csrf_token" {
			csrf = cookie.Value
			if cookie.HttpOnly {
				t.Fatal("CSRF cookie must be readable by the frontend")
			}
		}
		if strings.Contains(login.Body.String(), cookie.Value) {
			t.Fatal("token leaked in JSON")
		}
	}
	if csrf == "" || !strings.Contains(login.Body.String(), `"username":"tester"`) || strings.Contains(login.Body.String(), "password") {
		t.Fatal("invalid login response")
	}
	status(request("GET", "/api/v1/auth/me", "", "", "", cookies), 200)
	status(request("POST", "/api/v1/auth/logout", "", origin, "", cookies), 403)
	status(request("POST", "/api/v1/auth/logout", "", "https://attacker.test", csrf, cookies), 403)
	status(request("POST", "/api/v1/auth/logout", "", origin, strings.Repeat("x", 43), cookies), 403)
	logout := request("POST", "/api/v1/auth/logout", "", origin, csrf, cookies)
	status(logout, 200)
	if len(logout.Result().Cookies()) != 2 {
		t.Fatal("logout must clear both cookies")
	}
	for _, cookie := range logout.Result().Cookies() {
		if cookie.MaxAge != -1 {
			t.Fatal("logout did not clear cookies")
		}
	}
	status(request("GET", "/api/v1/auth/me", "", "", "", cookies), 401)

	// Rotation must return a new cookie and accept the previous one only during grace.
	login = request("POST", "/api/v1/auth/login", body, origin, "", nil)
	status(login, 200)
	cookies = login.Result().Cookies()
	if err := db.Model(&models.Session{}).Where("user_id = ? AND revoked_at IS NULL", user.ID).Update("rotated_at", time.Now().Add(-16*time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
	rotated := request("GET", "/api/v1/auth/me", "", "", "", cookies)
	status(rotated, 200)
	newCookies := rotated.Result().Cookies()
	if len(newCookies) != 1 || newCookies[0].Name != "sid" || newCookies[0].Value == cookies[0].Value {
		t.Fatal("rotated session cookie missing")
	}
	status(request("GET", "/api/v1/auth/me", "", "", "", cookies), 200)
	if err := db.Model(&models.Session{}).Where("user_id = ? AND revoked_at IS NULL", user.ID).Update("previous_valid_until", time.Now().Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	status(request("GET", "/api/v1/auth/me", "", "", "", cookies), 401)
	status(request("GET", "/api/v1/auth/me", "", "", "", newCookies), 200)
	if err := repo.RevokeUserSessions(ctx, user.ID, time.Now()); err != nil {
		t.Fatal(err)
	}
	status(request("GET", "/api/v1/auth/me", "", "", "", newCookies), 401)

	// Inactive users cannot log in.
	if err := db.Model(&user).Update("is_active", false).Error; err != nil {
		t.Fatal(err)
	}
	status(request("POST", "/api/v1/auth/login", body, origin, "", nil), 401)
	if err := db.Model(&user).Update("is_active", true).Error; err != nil {
		t.Fatal(err)
	}

	// Repository failures must be server errors, never invalid credentials.
	if err := db.Exec("DROP TABLE sessions").Error; err != nil {
		t.Fatal(err)
	}
	status(request("GET", "/api/v1/auth/me", "", "", "", newCookies), 500)
	if err := db.Exec("DROP TABLE users CASCADE").Error; err != nil {
		t.Fatal(err)
	}
	status(request("POST", "/api/v1/auth/login", body, origin, "", nil), 500)
}
