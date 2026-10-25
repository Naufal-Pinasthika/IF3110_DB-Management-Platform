package admin_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/admin"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/auth"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/models"
)

// A connection update must keep feat/dev's 400 validation response when it
// passes through the engine-aware helper shared with draft connection tests.
func TestConnectionUpdateValidationStatusByEngine(t *testing.T) {
	for _, engine := range []string{models.EnginePostgres, models.EngineMySQL} {
		t.Run(engine, func(t *testing.T) {
			repo := &mockRepository{getDatabaseFn: func(context.Context, string) (models.ManagedDatabase, error) {
				return models.ManagedDatabase{ID: "db1", Engine: engine}, nil
			}}
			handler := &admin.Handler{Service: newService(repo)}
			req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/databases/db1", strings.NewReader(`{"port":0}`))
			req.SetPathValue("dbId", "db1")
			req = req.WithContext(auth.SetValidatedPrincipalContext(req.Context(), adminPrincipal))
			response := httptest.NewRecorder()
			handler.UpdateManagedDatabase(response, req)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("invalid port returned %d, want 400: %s", response.Code, response.Body.String())
			}
		})
	}
}

type mergeConnectionTester struct {
	mockConnections
	tested      models.ManagedDatabase
	invalidated string
}

func (t *mergeConnectionTester) InvalidateTargetPools(id string) { t.invalidated = id }

func (t *mergeConnectionTester) TestConfiguration(_ context.Context, db models.ManagedDatabase) error {
	t.tested = db
	return t.err
}

func TestMySQLConnectionEditAndDraftKeepExistingWorkflows(t *testing.T) {
	existing := models.ManagedDatabase{
		ID: "db1", Engine: models.EngineMySQL, Host: "mysql", Port: 3306,
		DatabaseName: "inventory", SSLMode: "disable", IsEnabled: true,
		ReaderUsername: "reader", ReaderPasswordEnc: "enc:saved-reader",
		MigratorUsername: "migrator", MigratorPasswordEnc: "enc:saved-migrator",
	}
	var saved models.ManagedDatabase
	repo := &mockRepository{
		getDatabaseFn: func(context.Context, string) (models.ManagedDatabase, error) { return existing, nil },
		updateDatabaseFn: func(_ context.Context, db models.ManagedDatabase) (models.ManagedDatabase, error) {
			saved = db
			return db, nil
		},
	}
	svc := newService(repo)
	tester := &mergeConnectionTester{}
	svc.Connections = tester
	handler := &admin.Handler{Service: svc}
	request := func(body string, handle http.HandlerFunc) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/databases/db1", strings.NewReader(body))
		req.SetPathValue("dbId", "db1")
		req = req.WithContext(auth.SetValidatedPrincipalContext(req.Context(), adminPrincipal))
		response := httptest.NewRecorder()
		handle(response, req)
		if response.Code != http.StatusOK {
			t.Fatalf("status=%d: %s", response.Code, response.Body.String())
		}
		return response
	}
	request(`{"database_id":"db1","engine":"mysql","host":"draft-host","use_reader_credentials":true}`, handler.TestConnectionDraft)
	if tester.tested.TargetEngine() != models.EngineMySQL || tester.tested.Host != "draft-host" || tester.tested.MigratorPasswordEnc != "enc:saved-reader" {
		t.Fatalf("draft connection lost engine/settings/credentials: %+v", tester.tested)
	}
	if saved.ID != "" || tester.invalidated != "" {
		t.Fatal("draft test persisted settings or invalidated a saved pool")
	}
	response := request(`{"label":"Edited MySQL","port":3307,"use_reader_credentials":true}`, handler.UpdateManagedDatabase)
	if saved.TargetEngine() != models.EngineMySQL || saved.Port != 3307 || saved.MigratorUsername != "reader" || saved.MigratorPasswordEnc != "enc:saved-reader" || tester.invalidated != "db1" {
		t.Fatalf("connection edit lost engine/settings/credentials/pool invalidation: %+v, invalidated=%s", saved, tester.invalidated)
	}
	var result struct {
		Data struct {
			Engine string `json:"engine"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Data.Engine != "mysql" {
		t.Fatalf("updated connection engine=%q", result.Data.Engine)
	}
}
