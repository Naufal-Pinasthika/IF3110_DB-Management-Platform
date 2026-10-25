package testutil

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func RequireBrowser(t *testing.T) {
	t.Helper()
	if os.Getenv("REQUIRE_BROWSER_TESTS") != "1" {
		t.Skip("run make test-browser to enable Chromium tests")
	}
}

// RunBrowser keeps the test server alive while Node drives the real browser.
func RunBrowser(t *testing.T, script, origin string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", filepath.Join(Root(), "tests/browser", script))
	cmd.Env = append(os.Environ(), "PREVIEW_ORIGIN="+origin)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("browser %s: %v\n%s", script, err, output)
	}
	t.Logf("%s", output)
}
