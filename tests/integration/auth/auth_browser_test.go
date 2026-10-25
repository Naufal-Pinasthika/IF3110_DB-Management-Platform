package auth_test

import (
	"net/http/httptest"
	"testing"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/app"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/auth"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/models"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/tests/internal/testutil"
)

func TestAuthBrowserIntegration(t *testing.T) {
	testutil.RequireBrowser(t)
	db := testutil.Database(t)
	hash, err := auth.HashPassword("browser-password")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.User{Username: "browser-user", PasswordHash: hash, IsActive: true}).Error; err != nil {
		t.Fatal(err)
	}
	// Reserve a free port before composing the router so same-origin checks use
	// the exact browser origin. This starts no listener until the handler is ready.
	server := httptest.NewUnstartedServer(nil)
	t.Cleanup(server.Close)
	cfg := testutil.AuthConfig()
	cfg.PublicOrigin = "http://" + server.Listener.Addr().String()
	router, err := app.NewRouter(&auth.PostgresRepository{DB: db}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	server.Config.Handler = router
	server.Start()
	testutil.RunBrowser(t, "auth_test.mjs", server.URL)
}
