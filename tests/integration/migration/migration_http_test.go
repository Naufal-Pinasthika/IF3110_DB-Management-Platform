package migration_test

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/app"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/auth"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/config"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/models"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/store"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/tests/internal/testutil"
)

type targetConnParams struct {
	host     string
	port     int
	user     string
	password string
	sslMode  string
}

func parseTargetParams(t *testing.T) targetConnParams {
	t.Helper()
	raw := os.Getenv("TEST_DATABASE_URL")
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	port := 5432
	if p := u.Port(); p != "" {
		parsed, err := strconv.Atoi(p)
		if err != nil {
			t.Fatalf("parse port: %v", err)
		}
		port = parsed
	}
	pass, _ := u.User.Password()
	sslMode := u.Query().Get("sslmode")
	if sslMode == "" {
		sslMode = "disable"
	}
	return targetConnParams{
		host:     u.Hostname(),
		port:     port,
		user:     u.User.Username(),
		password: pass,
		sslMode:  sslMode,
	}
}

func createDisposableTargetDB(t *testing.T, prefix string) (string, *sql.DB, targetConnParams) {
	t.Helper()
	params := parseTargetParams(t)
	ctx := context.Background()
	adminGorm, err := store.OpenPlatform(ctx, os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatalf("open admin target pool: %v", err)
	}
	adminDB, err := adminGorm.DB()
	if err != nil {
		t.Fatalf("get admin sql.DB: %v", err)
	}

	var rnd [6]byte
	if _, err := rand.Read(rnd[:]); err != nil {
		t.Fatal(err)
	}
	dbName := fmt.Sprintf("%s_%x", prefix, rnd)
	if _, err := adminDB.Exec(fmt.Sprintf(`CREATE DATABASE "%s"`, dbName)); err != nil {
		_ = adminDB.Close()
		t.Fatalf("create disposable target db %s: %v", dbName, err)
	}

	targetDSN := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		url.QueryEscape(params.user), url.QueryEscape(params.password),
		params.host, params.port, url.QueryEscape(dbName), params.sslMode)
	targetGorm, err := store.OpenPlatform(ctx, targetDSN)
	if err != nil {
		_, _ = adminDB.Exec(fmt.Sprintf(`DROP DATABASE IF EXISTS "%s" WITH (FORCE)`, dbName))
		_ = adminDB.Close()
		t.Fatalf("open target db %s: %v", dbName, err)
	}
	targetConn, err := targetGorm.DB()
	if err != nil {
		_, _ = adminDB.Exec(fmt.Sprintf(`DROP DATABASE IF EXISTS "%s" WITH (FORCE)`, dbName))
		_ = adminDB.Close()
		t.Fatalf("get target sql.DB: %v", err)
	}

	t.Cleanup(func() {
		_ = targetConn.Close()
		if _, err := adminDB.Exec(fmt.Sprintf(`DROP DATABASE IF EXISTS "%s" WITH (FORCE)`, dbName)); err != nil {
			t.Errorf("drop target db %s: %v", dbName, err)
		}
		_ = adminDB.Close()
	})

	return dbName, targetConn, params
}

type testClient struct {
	t      *testing.T
	router http.Handler
	origin string
}

type userSession struct {
	cookies []*http.Cookie
	csrf    string
}

func (c *testClient) req(method, path string, payload any, sess *userSession) *httptest.ResponseRecorder {
	c.t.Helper()
	var body string
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			c.t.Fatalf("marshal payload: %v", err)
		}
		body = string(raw)
	}
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Origin", c.origin)
	if payload != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	if sess != nil {
		if sess.csrf != "" {
			r.Header.Set("X-CSRF-Token", sess.csrf)
		}
		for _, ck := range sess.cookies {
			r.AddCookie(ck)
		}
	}
	w := httptest.NewRecorder()
	c.router.ServeHTTP(w, r)
	return w
}

func (c *testClient) mustStatus(w *httptest.ResponseRecorder, want int) {
	c.t.Helper()
	if w.Code != want {
		c.t.Fatalf("status = %d, want %d: %s", w.Code, want, w.Body.String())
	}
}

func decodeData[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var env struct {
		Data T `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode %q: %v", w.Body.String(), err)
	}
	return env.Data
}

func (c *testClient) login(username, password string) *userSession {
	c.t.Helper()
	w := c.req("POST", "/api/v1/auth/login", api.LoginRequest{Username: username, Password: password}, nil)
	c.mustStatus(w, http.StatusOK)
	var csrf string
	for _, ck := range w.Result().Cookies() {
		if ck.Name == "csrf_token" {
			csrf = ck.Value
		}
	}
	if csrf == "" {
		c.t.Fatal("missing csrf_token cookie on login")
	}
	return &userSession{cookies: w.Result().Cookies(), csrf: csrf}
}

func setupMigrationEnv(t *testing.T, maxAttempts int) (*testClient, *userSession, targetConnParams, func()) {
	t.Helper()
	db := testutil.Database(t)

	adminHash, err := auth.HashPassword("admin-pass-123")
	if err != nil {
		t.Fatal(err)
	}
	adminUser := models.User{Username: "admin", PasswordHash: adminHash, IsActive: true, IsPlatformAdmin: true}
	if err := db.Create(&adminUser).Error; err != nil {
		t.Fatal(err)
	}

	cfg := testutil.AuthConfig()
	cfg.DBCredentialEncryptionKey = []byte("0123456789abcdef0123456789abcdef")
	cfg.QueryTimeout = 5 * time.Second
	cfg.Jobs = config.JobsConfig{
		Concurrency:       2,
		PollInterval:      25 * time.Millisecond,
		HeartbeatInterval: 200 * time.Millisecond,
		LeaseDuration:     2 * time.Second,
		ExecutionTimeout:  15 * time.Second,
		RetryBaseDelay:    25 * time.Millisecond,
		RetryMaxDelay:     80 * time.Millisecond,
		MaxAttempts:       maxAttempts,
		ShutdownTimeout:   5 * time.Second,
	}

	repo := &auth.PostgresRepository{DB: db}
	router, err := app.NewRouterWithJobs(repo, cfg, app.JobsIntegration{})
	if err != nil {
		t.Fatalf("build router: %v", err)
	}

	worker, err := app.NewMigrationWorker(db, cfg)
	if err != nil {
		t.Fatalf("build worker: %v", err)
	}
	workerCtx, cancelWorker := context.WithCancel(context.Background())
	worker.Start(workerCtx)

	stopWorker := func() {
		cancelWorker()
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = worker.Stop(stopCtx)
	}
	t.Cleanup(stopWorker)

	client := &testClient{t: t, router: router, origin: cfg.PublicOrigin}
	adminSess := client.login("admin", "admin-pass-123")
	params := parseTargetParams(t)
	return client, adminSess, params, stopWorker
}

func waitForMigrationStatus(t *testing.T, c *testClient, sess *userSession, dbID, migID string, want api.MigrationStatus) api.MigrationDetail {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var last api.MigrationDetail
	for time.Now().Before(deadline) {
		w := c.req("GET", "/api/v1/databases/"+dbID+"/migrations/"+migID, nil, sess)
		c.mustStatus(w, http.StatusOK)
		last = decodeData[api.MigrationDetail](t, w)
		if last.Status == want {
			return last
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for migration %s status %s (last=%s, error=%v)", migID, want, last.Status, last.ErrorSummary)
	return last
}

func waitForExecutionStatus(t *testing.T, c *testClient, sess *userSession, dbID, execID string, want string) api.MigrationExecutionSummary {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var last api.MigrationExecutionSummary
	for time.Now().Before(deadline) {
		w := c.req("GET", "/api/v1/databases/"+dbID+"/migration-history", nil, sess)
		c.mustStatus(w, http.StatusOK)
		list := decodeData[[]api.MigrationExecutionSummary](t, w)
		for _, item := range list {
			if item.ID == execID {
				last = item
				if item.Status == want {
					return item
				}
			}
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for execution %s status %s (last=%s, err=%v)", execID, want, last.Status, last.ErrorSummary)
	return last
}

func waitForJobBlocked(t *testing.T, c *testClient, sess *userSession, dbID, execID string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var lastStatus string
	for time.Now().Before(deadline) {
		w := c.req("GET", "/api/v1/databases/"+dbID+"/jobs", nil, sess)
		c.mustStatus(w, http.StatusOK)
		page := decodeData[struct {
			Jobs []struct {
				ExecutionID     string `json:"execution_id"`
				Status          string `json:"status"`
				DatabaseBlocked bool   `json:"database_blocked"`
			} `json:"jobs"`
		}](t, w)
		for _, j := range page.Jobs {
			if strings.EqualFold(j.ExecutionID, execID) {
				lastStatus = j.Status
				if j.Status == "BLOCKED" && j.DatabaseBlocked {
					return
				}
			}
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for job of execution %s to become BLOCKED (last=%s)", execID, lastStatus)
}

func registerTargetDatabase(t *testing.T, c *testClient, adminSess *userSession, label, dbName string, params targetConnParams) string {
	t.Helper()
	w := c.req("POST", "/api/v1/admin/databases", api.CreateDatabaseRequest{
		Engine:           models.EnginePostgres,
		Label:            label,
		Host:             params.host,
		Port:             params.port,
		DatabaseName:     dbName,
		SSLMode:          params.sslMode,
		ReaderUsername:   params.user,
		ReaderPassword:   params.password,
		MigratorUsername: params.user,
		MigratorPassword: params.password,
	}, adminSess)
	c.mustStatus(w, http.StatusCreated)
	return decodeData[api.ManagedDatabaseSummary](t, w).ID
}

// TestMigrationWorkflowIntegration covers end-to-end migration creation, plan analysis,
// multi-group leader approval/rejection, worker execution, rollback review/approval,
// forward review/approval, hide_rollback filtering, and access control against real PostgreSQL.
func TestMigrationWorkflowIntegration(t *testing.T) {
	c, adminSess, params, _ := setupMigrationEnv(t, 2)
	targetName, targetConn, _ := createDisposableTargetDB(t, "target_wf")

	createUser := func(username, password string) string {
		w := c.req("POST", "/api/v1/admin/users", api.CreateUserRequest{Username: username, Password: password}, adminSess)
		c.mustStatus(w, http.StatusCreated)
		return decodeData[api.AdminUser](t, w).ID
	}
	aliceID := createUser("alice", "alice-pass-123")
	bobID := createUser("bob", "bob-pass-123")
	carolID := createUser("carol", "carol-pass-123")
	_ = createUser("dave", "dave-pass-123")

	createGroup := func(name, leaderID string) string {
		w := c.req("POST", "/api/v1/admin/groups", api.CreateGroupRequest{Name: name, LeaderID: leaderID}, adminSess)
		c.mustStatus(w, http.StatusCreated)
		return decodeData[api.Group](t, w).ID
	}
	engGroupID := createGroup("engineering", aliceID)
	secGroupID := createGroup("security", bobID)

	c.mustStatus(c.req("PUT", "/api/v1/admin/groups/"+engGroupID+"/members/"+aliceID, nil, adminSess), http.StatusOK)
	c.mustStatus(c.req("PUT", "/api/v1/admin/groups/"+engGroupID+"/members/"+carolID, nil, adminSess), http.StatusOK)
	c.mustStatus(c.req("PUT", "/api/v1/admin/groups/"+secGroupID+"/members/"+bobID, nil, adminSess), http.StatusOK)

	dbID := registerTargetDatabase(t, c, adminSess, "target-wf", targetName, params)
	c.mustStatus(c.req("POST", "/api/v1/admin/databases/"+dbID+"/test-connection", nil, adminSess), http.StatusOK)
	c.mustStatus(c.req("PUT", "/api/v1/admin/groups/"+engGroupID+"/databases/"+dbID+"/access", api.SetGroupAccessRequest{AccessLevel: api.AccessViewEdit}, adminSess), http.StatusOK)
	c.mustStatus(c.req("PUT", "/api/v1/admin/groups/"+secGroupID+"/databases/"+dbID+"/access", api.SetGroupAccessRequest{AccessLevel: api.AccessViewOnly}, adminSess), http.StatusOK)

	aliceSess := c.login("alice", "alice-pass-123")
	bobSess := c.login("bob", "bob-pass-123")
	carolSess := c.login("carol", "carol-pass-123")
	daveSess := c.login("dave", "dave-pass-123")

	// 1. Access control checks: dave (no grant) and bob (VIEW_ONLY) cannot create migrations.
	c.mustStatus(c.req("GET", "/api/v1/databases/"+dbID+"/migrations", nil, daveSess), http.StatusForbidden)
	c.mustStatus(c.req("POST", "/api/v1/databases/"+dbID+"/migrations", api.CreateMigrationRequest{
		Name: "unauth", SQLText: "CREATE TABLE t (id INT);",
	}, bobSess), http.StatusForbidden)

	// 2. Carol (VIEW_EDIT) creates a migration draft and previews DDL explain against real target DB.
	downSQL := "DROP TABLE IF EXISTS public.widgets;"
	w := c.req("POST", "/api/v1/databases/"+dbID+"/migrations", api.CreateMigrationRequest{
		Name:    "create_widgets",
		SQLText: "CREATE TABLE public.widgets (id INT PRIMARY KEY, title TEXT NOT NULL);",
		SQLDown: &downSQL,
	}, carolSess)
	c.mustStatus(w, http.StatusCreated)
	migID := decodeData[api.MigrationCreated](t, w).ID

	w = c.req("POST", "/api/v1/databases/"+dbID+"/migrations/"+migID+"/explain", map[string]bool{"analyze": true}, carolSess)
	c.mustStatus(w, http.StatusOK)

	// 3. Submit for review, non-leader cannot approve, VIEW_ONLY leader Bob rejects with comment.
	c.mustStatus(c.req("POST", "/api/v1/databases/"+dbID+"/migrations/"+migID+"/submit-review", nil, carolSess), http.StatusOK)
	c.mustStatus(c.req("POST", "/api/v1/databases/"+dbID+"/migrations/"+migID+"/approve", api.DecideApprovalRequest{}, carolSess), http.StatusForbidden)

	rejectComment := "Add a created_at column"
	w = c.req("POST", "/api/v1/databases/"+dbID+"/migrations/"+migID+"/reject", api.DecideApprovalRequest{Comment: &rejectComment}, bobSess)
	c.mustStatus(w, http.StatusOK)
	if got := decodeData[api.MigrationDetail](t, w).Status; got != api.MigrationRejected {
		t.Fatalf("after reject status = %s, want REJECTED", got)
	}

	// 4. Carol updates rejected migration, resubmits, both leaders approve -> APPROVED.
	updatedSQL := "CREATE TABLE public.widgets (id INT PRIMARY KEY, title TEXT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now());"
	c.mustStatus(c.req("PATCH", "/api/v1/databases/"+dbID+"/migrations/"+migID, api.UpdateMigrationRequest{SQLText: &updatedSQL}, carolSess), http.StatusOK)
	c.mustStatus(c.req("POST", "/api/v1/databases/"+dbID+"/migrations/"+migID+"/submit-review", nil, carolSess), http.StatusOK)

	okComment := "LGTM"
	w = c.req("POST", "/api/v1/databases/"+dbID+"/migrations/"+migID+"/approve", api.DecideApprovalRequest{Comment: &okComment}, aliceSess)
	c.mustStatus(w, http.StatusOK)
	if got := decodeData[api.MigrationDetail](t, w).Status; got != api.MigrationPendingReview {
		t.Fatalf("after 1st approval status = %s, want PENDING_REVIEW", got)
	}
	w = c.req("POST", "/api/v1/databases/"+dbID+"/migrations/"+migID+"/approve", api.DecideApprovalRequest{Comment: &okComment}, bobSess)
	c.mustStatus(w, http.StatusOK)
	if got := decodeData[api.MigrationDetail](t, w).Status; got != api.MigrationApproved {
		t.Fatalf("after 2nd approval status = %s, want APPROVED", got)
	}

	// 4b. Q&A #2/#13: Adding a new group grant after APPROVED revokes status back to PENDING_REVIEW;
	// removing that grant leaves remaining groups intact; changing leader also revokes back to PENDING_REVIEW.
	extraID := createUser("eve", "eve-pass-123")
	extraGroupID := createGroup("platform-ops", extraID)
	c.mustStatus(c.req("PUT", "/api/v1/admin/groups/"+extraGroupID+"/members/"+extraID, nil, adminSess), http.StatusOK)
	c.mustStatus(c.req("PUT", "/api/v1/admin/groups/"+extraGroupID+"/databases/"+dbID+"/access", api.SetGroupAccessRequest{AccessLevel: api.AccessViewOnly}, adminSess), http.StatusOK)

	w = c.req("GET", "/api/v1/databases/"+dbID+"/migrations/"+migID, nil, carolSess)
	c.mustStatus(w, http.StatusOK)
	if got := decodeData[api.MigrationDetail](t, w).Status; got != api.MigrationPendingReview {
		t.Fatalf("after adding new group post-approval status = %s, want PENDING_REVIEW", got)
	}
	eveSess := c.login("eve", "eve-pass-123")
	w = c.req("POST", "/api/v1/databases/"+dbID+"/migrations/"+migID+"/approve", api.DecideApprovalRequest{Comment: &okComment}, eveSess)
	c.mustStatus(w, http.StatusOK)
	if got := decodeData[api.MigrationDetail](t, w).Status; got != api.MigrationApproved {
		t.Fatalf("after new group leader approved status = %s, want APPROVED", got)
	}
	// Clean up extra group grant so subsequent steps only need alice + bob.
	c.mustStatus(c.req("DELETE", "/api/v1/admin/groups/"+extraGroupID+"/databases/"+dbID+"/access", nil, adminSess), http.StatusNoContent)

	// 5. Execute asynchronously via worker and verify real table creation in targetDB.
	w = c.req("POST", "/api/v1/databases/"+dbID+"/migrations/"+migID+"/execute", nil, carolSess)
	c.mustStatus(w, http.StatusOK)
	applied := waitForMigrationStatus(t, c, carolSess, dbID, migID, api.MigrationApplied)
	if applied.Version == nil || *applied.Version != 1 {
		t.Fatalf("expected version 1, got %v", applied.Version)
	}
	if !applied.CanRequestRollback || applied.CanRollback {
		t.Fatalf("expected CanRequestRollback=true, CanRollback=false, got %+v", applied)
	}

	var exists bool
	if err := targetConn.QueryRow(`SELECT to_regclass('public.widgets') IS NOT NULL`).Scan(&exists); err != nil || !exists {
		t.Fatalf("expected public.widgets table in target DB (exists=%v, err=%v)", exists, err)
	}

	// 6. Direct rollback without approval is rejected; rollback review rejection restores APPLIED.
	c.mustStatus(c.req("POST", "/api/v1/databases/"+dbID+"/migrations/"+migID+"/rollback", nil, carolSess), http.StatusConflict)
	c.mustStatus(c.req("POST", "/api/v1/databases/"+dbID+"/migrations/"+migID+"/submit-rollback-review", nil, carolSess), http.StatusOK)
	c.mustStatus(c.req("POST", "/api/v1/databases/"+dbID+"/migrations/"+migID+"/reject", api.DecideApprovalRequest{Comment: &rejectComment}, bobSess), http.StatusOK)

	w = c.req("GET", "/api/v1/databases/"+dbID+"/migrations/"+migID, nil, carolSess)
	c.mustStatus(w, http.StatusOK)
	if got := decodeData[api.MigrationDetail](t, w).Status; got != api.MigrationApplied {
		t.Fatalf("after rollback rejection status = %s, want APPLIED", got)
	}

	// 7. Full rollback approval -> ROLLBACK_APPROVED -> Execute rollback -> ROLLED_BACK.
	c.mustStatus(c.req("POST", "/api/v1/databases/"+dbID+"/migrations/"+migID+"/submit-rollback-review", nil, carolSess), http.StatusOK)
	c.mustStatus(c.req("POST", "/api/v1/databases/"+dbID+"/migrations/"+migID+"/approve", api.DecideApprovalRequest{}, aliceSess), http.StatusOK)
	w = c.req("POST", "/api/v1/databases/"+dbID+"/migrations/"+migID+"/approve", api.DecideApprovalRequest{}, bobSess)
	c.mustStatus(w, http.StatusOK)
	rbReady := decodeData[api.MigrationDetail](t, w)
	if rbReady.Status != api.MigrationRollbackApproved || !rbReady.CanRollback {
		t.Fatalf("expected ROLLBACK_APPROVED + CanRollback, got status=%s canRollback=%v", rbReady.Status, rbReady.CanRollback)
	}

	c.mustStatus(c.req("POST", "/api/v1/databases/"+dbID+"/migrations/"+migID+"/rollback", nil, carolSess), http.StatusOK)
	rolledBack := waitForMigrationStatus(t, c, carolSess, dbID, migID, api.MigrationRolledBack)
	if !rolledBack.CanRequestForward {
		t.Fatal("expected CanRequestForward=true on ROLLED_BACK migration")
	}
	if err := targetConn.QueryRow(`SELECT to_regclass('public.widgets') IS NOT NULL`).Scan(&exists); err != nil || exists {
		t.Fatalf("expected public.widgets dropped after rollback (exists=%v, err=%v)", exists, err)
	}

	// 8. Forward without approval is rejected; submit-forward-review -> approve -> forward -> APPLIED v2.
	c.mustStatus(c.req("POST", "/api/v1/databases/"+dbID+"/migrations/"+migID+"/forward", nil, carolSess), http.StatusConflict)
	c.mustStatus(c.req("POST", "/api/v1/databases/"+dbID+"/migrations/"+migID+"/submit-forward-review", nil, carolSess), http.StatusOK)
	c.mustStatus(c.req("POST", "/api/v1/databases/"+dbID+"/migrations/"+migID+"/approve", api.DecideApprovalRequest{}, aliceSess), http.StatusOK)
	c.mustStatus(c.req("POST", "/api/v1/databases/"+dbID+"/migrations/"+migID+"/approve", api.DecideApprovalRequest{}, bobSess), http.StatusOK)
	c.mustStatus(c.req("POST", "/api/v1/databases/"+dbID+"/migrations/"+migID+"/forward", nil, carolSess), http.StatusOK)

	forwarded := waitForMigrationStatus(t, c, carolSess, dbID, migID, api.MigrationApplied)
	if forwarded.Version == nil || *forwarded.Version != 2 {
		t.Fatalf("expected re-applied version 2 after rollback (monotonic history), got %v", forwarded.Version)
	}
	if err := targetConn.QueryRow(`SELECT to_regclass('public.widgets') IS NOT NULL`).Scan(&exists); err != nil || !exists {
		t.Fatalf("expected public.widgets recreated after forward (exists=%v, err=%v)", exists, err)
	}

	// 9. Verify execution history & hide_rollback=true filtering.
	w = c.req("GET", "/api/v1/databases/"+dbID+"/migration-history", nil, bobSess)
	c.mustStatus(w, http.StatusOK)
	fullHist := decodeData[[]api.MigrationExecutionSummary](t, w)
	if len(fullHist) != 3 {
		t.Fatalf("expected 3 execution history rows, got %d", len(fullHist))
	}

	w = c.req("GET", "/api/v1/databases/"+dbID+"/migration-history?hide_rollback=true", nil, bobSess)
	c.mustStatus(w, http.StatusOK)
	filteredHist := decodeData[[]api.MigrationExecutionSummary](t, w)
	if len(filteredHist) != 1 || filteredHist[0].ActionType != "FORWARD" {
		t.Fatalf("expected only final FORWARD row with hide_rollback=true, got %+v", filteredHist)
	}
}

// TestBatchCheckpointRecoveryIntegration tests real PostgreSQL batch execution failing mid-way
// (PARTIAL), resuming from checkpoint (RETRY), and compensating committed batches (REVERT_BATCH).
func TestBatchCheckpointRecoveryIntegration(t *testing.T) {
	c, adminSess, params, _ := setupMigrationEnv(t, 1)
	targetName, targetConn, _ := createDisposableTargetDB(t, "target_batch")

	if _, err := targetConn.Exec(`
		CREATE TABLE public.batch_items (
			id INT PRIMARY KEY,
			divisor INT NOT NULL,
			updated INT NOT NULL DEFAULT 0
		);
		INSERT INTO public.batch_items (id, divisor, updated) VALUES
			(1, 1, 0),
			(2, 1, 0),
			(3, 1, 0),
			(4, 0, 0),
			(5, 1, 0);
	`); err != nil {
		t.Fatalf("seed target batch_items: %v", err)
	}

	w := c.req("POST", "/api/v1/admin/users", api.CreateUserRequest{Username: "lead", Password: "lead-pass-123"}, adminSess)
	c.mustStatus(w, http.StatusCreated)
	leadID := decodeData[api.AdminUser](t, w).ID

	w = c.req("POST", "/api/v1/admin/groups", api.CreateGroupRequest{Name: "batchteam", LeaderID: leadID}, adminSess)
	c.mustStatus(w, http.StatusCreated)
	groupID := decodeData[api.Group](t, w).ID
	c.mustStatus(c.req("PUT", "/api/v1/admin/groups/"+groupID+"/members/"+leadID, nil, adminSess), http.StatusOK)

	dbID := registerTargetDatabase(t, c, adminSess, "target-batch", targetName, params)
	c.mustStatus(c.req("PUT", "/api/v1/admin/groups/"+groupID+"/databases/"+dbID+"/access", api.SetGroupAccessRequest{AccessLevel: api.AccessViewEdit}, adminSess), http.StatusOK)
	leadSess := c.login("lead", "lead-pass-123")

	batchAttr := "id"
	batchSize := 2
	sqlUp := `UPDATE public.batch_items SET updated = 1 / divisor WHERE id IN (SELECT id FROM public.batch_items WHERE id > {{CURSOR}} ORDER BY id ASC LIMIT {{BATCH_SIZE}});`
	sqlDown := `UPDATE public.batch_items SET updated = 0 WHERE id IN (SELECT id FROM public.batch_items WHERE id <= {{CURSOR}} AND updated = 1 ORDER BY id DESC LIMIT {{BATCH_SIZE}});`

	// Create and approve batch migration #1 (for Resume test).
	w = c.req("POST", "/api/v1/databases/"+dbID+"/migrations", api.CreateMigrationRequest{
		Name:           "batch_resume_test",
		SQLText:        sqlUp,
		SQLDown:        &sqlDown,
		MigrationType:  "BATCH",
		BatchAttribute: &batchAttr,
		BatchSize:      &batchSize,
	}, leadSess)
	c.mustStatus(w, http.StatusCreated)
	migResumeID := decodeData[api.MigrationCreated](t, w).ID
	c.mustStatus(c.req("POST", "/api/v1/databases/"+dbID+"/migrations/"+migResumeID+"/submit-review", nil, leadSess), http.StatusOK)
	c.mustStatus(c.req("POST", "/api/v1/databases/"+dbID+"/migrations/"+migResumeID+"/approve", api.DecideApprovalRequest{}, leadSess), http.StatusOK)

	w = c.req("POST", "/api/v1/databases/"+dbID+"/migrations/"+migResumeID+"/execute", nil, leadSess)
	c.mustStatus(w, http.StatusOK)
	exec1ID := decodeData[api.MigrationExecuted](t, w).ExecutionID

	// Batch 1 (id 1,2) commits, batch 2 fails on divisor=0 at id=4 -> PARTIAL with cursor="2".
	partialExec := waitForExecutionStatus(t, c, leadSess, dbID, exec1ID, "PARTIAL")
	waitForJobBlocked(t, c, leadSess, dbID, exec1ID)
	if partialExec.CommittedBatches != 1 || partialExec.CommittedRows != 2 {
		t.Fatalf("expected 1 batch / 2 rows committed before failure, got %d / %d", partialExec.CommittedBatches, partialExec.CommittedRows)
	}

	var updatedCount int
	if err := targetConn.QueryRow(`SELECT COUNT(*) FROM public.batch_items WHERE updated = 1`).Scan(&updatedCount); err != nil || updatedCount != 2 {
		t.Fatalf("expected exactly 2 updated rows persisted from batch 1, got %d (err=%v)", updatedCount, err)
	}

	// Fix row 4 divisor and resume from checkpoint -> remaining rows 3,4,5 complete!
	if _, err := targetConn.Exec(`UPDATE public.batch_items SET divisor = 1 WHERE id = 4`); err != nil {
		t.Fatal(err)
	}
	w = c.req("POST", "/api/v1/databases/"+dbID+"/executions/"+exec1ID+"/resume", nil, leadSess)
	c.mustStatus(w, http.StatusOK)
	resumeExecID := decodeData[api.MigrationExecuted](t, w).ExecutionID
	waitForExecutionStatus(t, c, leadSess, dbID, resumeExecID, "SUCCEEDED")
	waitForMigrationStatus(t, c, leadSess, dbID, migResumeID, api.MigrationApplied)

	if err := targetConn.QueryRow(`SELECT COUNT(*) FROM public.batch_items WHERE updated = 1`).Scan(&updatedCount); err != nil || updatedCount != 5 {
		t.Fatalf("expected all 5 rows updated after resume, got %d (err=%v)", updatedCount, err)
	}

	// A stale retry against the original partial execution must be rejected
	// without creating an unreconcilable UNKNOWN history row.
	w = c.req("POST", "/api/v1/databases/"+dbID+"/executions/"+exec1ID+"/resume", nil, leadSess)
	c.mustStatus(w, http.StatusConflict)
	w = c.req("GET", "/api/v1/databases/"+dbID+"/migration-history", nil, leadSess)
	c.mustStatus(w, http.StatusOK)
	rejectedHistory := decodeData[[]api.MigrationExecutionSummary](t, w)
	rejectedAsFailed := false
	for _, item := range rejectedHistory {
		if item.ActionType == "RETRY" && item.RolledBackExecutionID != nil && *item.RolledBackExecutionID == exec1ID &&
			item.Status == "FAILED" && item.JobStatus == "" && item.ErrorSummary != nil && strings.Contains(*item.ErrorSummary, "queue rejected execution") {
			rejectedAsFailed = true
			break
		}
	}
	if !rejectedAsFailed {
		t.Fatalf("expected stale retry to be recorded as FAILED without a job, history=%+v", rejectedHistory)
	}

	// Reset table with divisor=0 on id=4 to test CancelBatch (REVERT_BATCH compensation).
	if _, err := targetConn.Exec(`UPDATE public.batch_items SET updated = 0; UPDATE public.batch_items SET divisor = 0 WHERE id = 4;`); err != nil {
		t.Fatal(err)
	}

	w = c.req("POST", "/api/v1/databases/"+dbID+"/migrations", api.CreateMigrationRequest{
		Name:           "batch_cancel_test",
		SQLText:        sqlUp,
		SQLDown:        &sqlDown,
		MigrationType:  "BATCH",
		BatchAttribute: &batchAttr,
		BatchSize:      &batchSize,
	}, leadSess)
	c.mustStatus(w, http.StatusCreated)
	migCancelID := decodeData[api.MigrationCreated](t, w).ID
	c.mustStatus(c.req("POST", "/api/v1/databases/"+dbID+"/migrations/"+migCancelID+"/submit-review", nil, leadSess), http.StatusOK)
	c.mustStatus(c.req("POST", "/api/v1/databases/"+dbID+"/migrations/"+migCancelID+"/approve", api.DecideApprovalRequest{}, leadSess), http.StatusOK)

	w = c.req("POST", "/api/v1/databases/"+dbID+"/migrations/"+migCancelID+"/execute", nil, leadSess)
	c.mustStatus(w, http.StatusOK)
	execCancelTargetID := decodeData[api.MigrationExecuted](t, w).ExecutionID
	waitForExecutionStatus(t, c, leadSess, dbID, execCancelTargetID, "PARTIAL")
	waitForJobBlocked(t, c, leadSess, dbID, execCancelTargetID)

	// Cancel batch -> REVERT_BATCH runs SQLDown in reverse order and sets updated back to 0!
	w = c.req("POST", "/api/v1/databases/"+dbID+"/executions/"+execCancelTargetID+"/cancel-batch", nil, leadSess)
	c.mustStatus(w, http.StatusOK)
	revertExecID := decodeData[api.MigrationExecuted](t, w).ExecutionID
	waitForExecutionStatus(t, c, leadSess, dbID, revertExecID, "SUCCEEDED")
	waitForMigrationStatus(t, c, leadSess, dbID, migCancelID, api.MigrationRolledBack)

	if err := targetConn.QueryRow(`SELECT COUNT(*) FROM public.batch_items WHERE updated = 1`).Scan(&updatedCount); err != nil || updatedCount != 0 {
		t.Fatalf("expected 0 updated rows after batch compensation, got %d (err=%v)", updatedCount, err)
	}
}

// TestParallelDatabaseConcurrencyIntegration verifies that different databases execute
// migrations in parallel while migrations on the same database serialize strictly.
func TestParallelDatabaseConcurrencyIntegration(t *testing.T) {
	c, adminSess, params, _ := setupMigrationEnv(t, 2)
	targetAName, targetAConn, _ := createDisposableTargetDB(t, "target_con_a")
	targetBName, targetBConn, _ := createDisposableTargetDB(t, "target_con_b")

	w := c.req("POST", "/api/v1/admin/users", api.CreateUserRequest{Username: "ops", Password: "ops-pass-123"}, adminSess)
	c.mustStatus(w, http.StatusCreated)
	opsID := decodeData[api.AdminUser](t, w).ID

	w = c.req("POST", "/api/v1/admin/groups", api.CreateGroupRequest{Name: "opsgroup", LeaderID: opsID}, adminSess)
	c.mustStatus(w, http.StatusCreated)
	groupID := decodeData[api.Group](t, w).ID
	c.mustStatus(c.req("PUT", "/api/v1/admin/groups/"+groupID+"/members/"+opsID, nil, adminSess), http.StatusOK)

	dbAID := registerTargetDatabase(t, c, adminSess, "target-con-a", targetAName, params)
	dbBID := registerTargetDatabase(t, c, adminSess, "target-con-b", targetBName, params)
	c.mustStatus(c.req("PUT", "/api/v1/admin/groups/"+groupID+"/databases/"+dbAID+"/access", api.SetGroupAccessRequest{AccessLevel: api.AccessViewEdit}, adminSess), http.StatusOK)
	c.mustStatus(c.req("PUT", "/api/v1/admin/groups/"+groupID+"/databases/"+dbBID+"/access", api.SetGroupAccessRequest{AccessLevel: api.AccessViewEdit}, adminSess), http.StatusOK)

	opsSess := c.login("ops", "ops-pass-123")

	createAndApprove := func(dbID, name, sqlText string) string {
		w := c.req("POST", "/api/v1/databases/"+dbID+"/migrations", api.CreateMigrationRequest{Name: name, SQLText: sqlText}, opsSess)
		c.mustStatus(w, http.StatusCreated)
		id := decodeData[api.MigrationCreated](t, w).ID
		c.mustStatus(c.req("POST", "/api/v1/databases/"+dbID+"/migrations/"+id+"/submit-review", nil, opsSess), http.StatusOK)
		c.mustStatus(c.req("POST", "/api/v1/databases/"+dbID+"/migrations/"+id+"/approve", api.DecideApprovalRequest{}, opsSess), http.StatusOK)
		return id
	}

	slowAID1 := createAndApprove(dbAID, "slow_a1", "SELECT pg_sleep(1.5); CREATE TABLE public.a1 (id INT);")
	fastAID2 := createAndApprove(dbAID, "fast_a2", "CREATE TABLE public.a2 (id INT);")
	fastBID1 := createAndApprove(dbBID, "fast_b1", "CREATE TABLE public.b1 (id INT);")

	c.mustStatus(c.req("POST", "/api/v1/databases/"+dbAID+"/migrations/"+slowAID1+"/execute", nil, opsSess), http.StatusOK)
	c.mustStatus(c.req("POST", "/api/v1/databases/"+dbAID+"/migrations/"+fastAID2+"/execute", nil, opsSess), http.StatusOK)
	c.mustStatus(c.req("POST", "/api/v1/databases/"+dbBID+"/migrations/"+fastBID1+"/execute", nil, opsSess), http.StatusOK)

	bDone := waitForMigrationStatus(t, c, opsSess, dbBID, fastBID1, api.MigrationApplied)
	a1Done := waitForMigrationStatus(t, c, opsSess, dbAID, slowAID1, api.MigrationApplied)
	a2Done := waitForMigrationStatus(t, c, opsSess, dbAID, fastAID2, api.MigrationApplied)

	if bDone.ExecutedAt == nil || a1Done.ExecutedAt == nil || a2Done.ExecutedAt == nil {
		t.Fatalf("missing ExecutedAt timestamps: b=%v a1=%v a2=%v", bDone.ExecutedAt, a1Done.ExecutedAt, a2Done.ExecutedAt)
	}
	if !bDone.ExecutedAt.Before(*a1Done.ExecutedAt) {
		t.Fatalf("expected dbB fast migration (%v) to complete before dbA slow migration (%v)", bDone.ExecutedAt, a1Done.ExecutedAt)
	}
	if a2Done.Version == nil || a1Done.Version == nil || *a2Done.Version <= *a1Done.Version {
		t.Fatalf("expected dbA migrations to serialize sequentially: v1=%v v2=%v", a1Done.Version, a2Done.Version)
	}

	var exists bool
	if err := targetAConn.QueryRow(`SELECT to_regclass('public.a2') IS NOT NULL`).Scan(&exists); err != nil || !exists {
		t.Fatalf("expected public.a2 in targetA (exists=%v, err=%v)", exists, err)
	}
	if err := targetBConn.QueryRow(`SELECT to_regclass('public.b1') IS NOT NULL`).Scan(&exists); err != nil || !exists {
		t.Fatalf("expected public.b1 in targetB (exists=%v, err=%v)", exists, err)
	}
}
