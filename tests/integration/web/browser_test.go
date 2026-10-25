package web_test

import (
	"net/http/httptest"
	"testing"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/tests/internal/testutil"
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/web"
)

func TestFrontendBrowserFixtures(t *testing.T) {
	testutil.RequireBrowser(t)
	pages, err := web.New()
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(pages)
	t.Cleanup(server.Close)
	testutil.RunBrowser(t, "fixtures_test.mjs", server.URL)
}
