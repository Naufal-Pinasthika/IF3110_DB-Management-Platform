package admin_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/app"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/auth"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/models"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/tests/internal/testutil"
)

// dataID extracts the "id" field from a DataResponse envelope.
func dataID(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var resp struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode %q: %v", w.Body.String(), err)
	}
	if resp.Data.ID == "" {
		t.Fatalf("no id in response: %s", w.Body.String())
	}
	return resp.Data.ID
}

// TestAdminAndAccessHTTPIntegration drives the full router against a real
// PostgreSQL and checks authentication, admin authorization, validation error
// codes, promote/demote, and group-based database access enforcement.
func TestAdminAndAccessHTTPIntegration(t *testing.T) {
	db := testutil.Database(t)

	adminHash, err := auth.HashPassword("admin-pass-123")
	if err != nil {
		t.Fatal(err)
	}
	userHash, err := auth.HashPassword("user-pass-123")
	if err != nil {
		t.Fatal(err)
	}

	adminUser := models.User{Username: "admin", PasswordHash: adminHash, IsActive: true, IsPlatformAdmin: true}
	if err := db.Create(&adminUser).Error; err != nil {
		t.Fatal(err)
	}
	bob := models.User{Username: "bob", PasswordHash: userHash, IsActive: true}
	if err := db.Create(&bob).Error; err != nil {
		t.Fatal(err)
	}
	dave := models.User{Username: "dave", PasswordHash: userHash, IsActive: true}
	if err := db.Create(&dave).Error; err != nil {
		t.Fatal(err)
	}

	repo := &auth.PostgresRepository{DB: db}
	cfg := testutil.AuthConfig()
	router, err := app.NewRouter(repo, cfg)
	if err != nil {
		t.Fatal(err)
	}
	origin := cfg.PublicOrigin

	request := func(method, path, body, csrf string, cookies []*http.Cookie) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Origin", origin)
		if csrf != "" {
			r.Header.Set("X-CSRF-Token", csrf)
		}
		for _, c := range cookies {
			r.AddCookie(c)
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
	login := func(username, password string) ([]*http.Cookie, string) {
		t.Helper()
		w := request("POST", "/api/v1/auth/login", `{"username":"`+username+`","password":"`+password+`"}`, "", nil)
		status(w, 200)
		var csrf string
		for _, c := range w.Result().Cookies() {
			if c.Name == "csrf_token" {
				csrf = c.Value
			}
		}
		if csrf == "" {
			t.Fatal("no csrf token in login cookies")
		}
		return w.Result().Cookies(), csrf
	}

	adminCookies, adminCSRF := login("admin", "admin-pass-123")
	bobCookies, _ := login("bob", "user-pass-123")
	daveCookies, _ := login("dave", "user-pass-123")

	// Unauthenticated admin endpoint is rejected.
	status(request("GET", "/api/v1/admin/users", "", "", nil), 401)

	// Non-admin is forbidden from admin endpoints.
	status(request("GET", "/api/v1/admin/users", "", "", bobCookies), 403)

	// Admin can list accounts.
	w := request("GET", "/api/v1/admin/users", "", "", adminCookies)
	status(w, 200)
	for _, name := range []string{`"username":"admin"`, `"username":"bob"`, `"username":"dave"`} {
		if !strings.Contains(w.Body.String(), name) {
			t.Fatalf("users list missing %s: %s", name, w.Body.String())
		}
	}

	// Validation errors must be 400, not 500.
	status(request("POST", "/api/v1/admin/users", `{"username":"carol","password":"short"}`, adminCSRF, adminCookies), 400)

	// Missing CSRF token is rejected.
	status(request("POST", "/api/v1/admin/users", `{"username":"carol","password":"carol-pass-123"}`, "", adminCookies), 403)

	// Admin can create a user.
	w = request("POST", "/api/v1/admin/users", `{"username":"carol","password":"carol-pass-123"}`, adminCSRF, adminCookies)
	status(w, 201)

	// Admin can promote an existing user to platform admin.
	w = request("PATCH", "/api/v1/admin/users/"+bob.ID, `{"is_platform_admin":true}`, adminCSRF, adminCookies)
	status(w, 200)
	if !strings.Contains(w.Body.String(), `"is_platform_admin":true`) {
		t.Fatalf("promote failed: %s", w.Body.String())
	}

	// Admin can demote a user back.
	w = request("PATCH", "/api/v1/admin/users/"+bob.ID, `{"is_platform_admin":false}`, adminCSRF, adminCookies)
	status(w, 200)
	if !strings.Contains(w.Body.String(), `"is_platform_admin":false`) {
		t.Fatalf("demote failed: %s", w.Body.String())
	}

	// Group + database + grant flow.
	w = request("POST", "/api/v1/admin/groups", `{"name":"analysts"}`, adminCSRF, adminCookies)
	status(w, 201)
	groupID := dataID(t, w)

	w = request("POST", "/api/v1/admin/databases", `{"label":"analytics"}`, adminCSRF, adminCookies)
	status(w, 201)
	dbID := dataID(t, w)

	status(request("PUT", "/api/v1/admin/groups/"+groupID+"/members/"+bob.ID, "", adminCSRF, adminCookies), 200)
	status(request("PUT", "/api/v1/admin/groups/"+groupID+"/databases/"+dbID+"/access", `{"access_level":"VIEW_ONLY"}`, adminCSRF, adminCookies), 200)

	// Platform admin does NOT bypass group grants.
	w = request("GET", "/api/v1/databases", "", "", adminCookies)
	status(w, 200)
	if strings.Contains(w.Body.String(), "analytics") {
		t.Fatalf("admin saw a database without a grant: %s", w.Body.String())
	}

	// The granted user sees the database with VIEW_ONLY effective access.
	bobCookies, bobCSRF := login("bob", "user-pass-123")
	w = request("GET", "/api/v1/databases", "", "", bobCookies)
	status(w, 200)
	if !strings.Contains(w.Body.String(), "analytics") || !strings.Contains(w.Body.String(), "VIEW_ONLY") {
		t.Fatalf("bob should see analytics with VIEW_ONLY: %s", w.Body.String())
	}

	// VIEW_ONLY is not enough to create a migration (VIEW_EDIT required).
	status(request("POST", "/api/v1/databases/"+dbID+"/migrations", `{"name":"m1","sql_text":"SELECT 1"}`, bobCSRF, bobCookies), 403)

	// Multi-group effective access: VIEW_EDIT wins over VIEW_ONLY.
	w = request("POST", "/api/v1/admin/groups", `{"name":"editors"}`, adminCSRF, adminCookies)
	status(w, 201)
	editorGroupID := dataID(t, w)
	status(request("PUT", "/api/v1/admin/groups/"+editorGroupID+"/members/"+bob.ID, "", adminCSRF, adminCookies), 200)
	status(request("PUT", "/api/v1/admin/groups/"+editorGroupID+"/databases/"+dbID+"/access", `{"access_level":"VIEW_EDIT"}`, adminCSRF, adminCookies), 200)

	bobCookies, _ = login("bob", "user-pass-123")
	w = request("GET", "/api/v1/databases", "", "", bobCookies)
	status(w, 200)
	if !strings.Contains(w.Body.String(), "analytics") || !strings.Contains(w.Body.String(), "VIEW_EDIT") {
		t.Fatalf("bob should see analytics with VIEW_EDIT (multi-group merge): %s", w.Body.String())
	}

	// A user with no grants sees nothing.
	w = request("GET", "/api/v1/databases", "", "", daveCookies)
	status(w, 200)
	if strings.Contains(w.Body.String(), "analytics") {
		t.Fatalf("dave saw a database without a grant: %s", w.Body.String())
	}

	// Direct database access without a grant is forbidden.
	status(request("GET", "/api/v1/databases/"+dbID+"/schemas", "", "", daveCookies), 403)
}
