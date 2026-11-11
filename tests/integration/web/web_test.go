package web_test

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/web"
)

func spaHandler(t *testing.T) http.Handler {
	t.Helper()
	handler, err := web.New()
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func serve(handler http.Handler, method, path string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(method, path, nil))
	return response
}

func TestPagesRender(t *testing.T) {
	handler := spaHandler(t)
	index := serve(handler, http.MethodGet, "/")
	if !strings.Contains(index.Body.String(), `<div id="root"></div>`) {
		t.Fatal("missing React mount point; run npm --prefix frontend run build")
	}
	if !strings.Contains(index.Body.String(), `<script type="module"`) || !strings.Contains(index.Body.String(), `/assets/`) {
		t.Fatal("missing built SPA module assets")
	}
	for _, path := range []string{
		"/login", "/databases", "/databases/db-1", "/databases/db-1/query",
		"/databases/db-1/schemas/public/tables/users", "/databases/db-1/migrations",
		"/databases/db-1/migrations/new", "/databases/db-1/migrations/m-1",
		"/admin/users", "/admin/groups", "/admin/databases",
		"/databases/db.example/query?tab=plan", "/unknown-client-route",
	} {
		t.Run(path, func(t *testing.T) {
			response := serve(handler, http.MethodGet, path)
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d: %s", response.Code, response.Body.String())
			}
			if response.Body.String() != index.Body.String() {
				t.Fatal("deep links must serve the same SPA shell; React Router resolves the page")
			}
			if !strings.HasPrefix(response.Header().Get("Content-Type"), "text/html") {
				t.Fatal("SPA shell is not HTML")
			}
			if response.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("SPA shell must not be cached")
			}
		})
	}
}

func TestRouteBoundaries(t *testing.T) {
	handler := spaHandler(t)
	for _, tc := range []struct {
		method, path string
		want         int
	}{
		{"GET", "/", 200}, {"HEAD", "/login", 200}, {"GET", "/missing", 200},
		{"POST", "/login", 405}, {"PUT", "/admin/users", 405},
		{"GET", "/api", 404}, {"GET", "/api/v1/missing", 404},
		{"GET", "/assets", 404}, {"GET", "/assets/missing.js", 404},
		{"GET", "/missing.json", 404}, {"GET", "/.gitkeep", 404},
		{"GET", "/.env", 404}, {"GET", "/static/app.css", 404},
		{"GET", "/static/app.js", 404}, {"GET", "/static/missing", 404},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			response := serve(handler, tc.method, tc.path)
			if response.Code != tc.want {
				t.Fatalf("status = %d, want %d", response.Code, tc.want)
			}
			if tc.want != http.StatusOK && strings.Contains(response.Body.String(), `<div id="root">`) {
				t.Fatal("resource errors must not fall back to the SPA shell")
			}
			if tc.want == http.StatusMethodNotAllowed && response.Header().Get("Allow") != "GET, HEAD" {
				t.Fatal("unsupported methods must advertise GET and HEAD")
			}
		})
	}
}

func TestSPAAssets(t *testing.T) {
	handler := spaHandler(t)
	index := serve(handler, http.MethodGet, "/")
	assets := regexp.MustCompile(`(?:src|href)="(/assets/[^" ]+\.(js|css))"`).FindAllStringSubmatch(index.Body.String(), -1)
	found := map[string]bool{}
	for _, asset := range assets {
		path, kind := asset[1], asset[2]
		found[kind] = true
		t.Run(path, func(t *testing.T) {
			response := serve(handler, http.MethodGet, path)
			if response.Code != http.StatusOK || response.Body.Len() == 0 {
				t.Fatalf("bundle unavailable: status=%d bytes=%d", response.Code, response.Body.Len())
			}
			contentType := response.Header().Get("Content-Type")
			if kind == "js" && !strings.Contains(contentType, "javascript") || kind == "css" && !strings.HasPrefix(contentType, "text/css") {
				t.Fatalf("incorrect bundle content type: %s", contentType)
			}
			if response.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
				t.Fatal("hashed bundles must be cached as immutable")
			}
			head := serve(handler, http.MethodHead, path)
			if head.Code != http.StatusOK || head.Body.Len() != 0 || head.Header().Get("Content-Type") != contentType {
				t.Fatal("HEAD must return bundle headers without its body")
			}
		})
	}
	if !found["js"] || !found["css"] {
		t.Fatal("SPA must reference both JavaScript and CSS bundles")
	}
}

func TestSPAHead(t *testing.T) {
	handler := spaHandler(t)
	response := serve(handler, http.MethodHead, "/admin/users")
	if response.Code != http.StatusOK || response.Body.Len() != 0 {
		t.Fatalf("HEAD shell: status=%d bytes=%d", response.Code, response.Body.Len())
	}
	if !strings.HasPrefix(response.Header().Get("Content-Type"), "text/html") || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("HEAD must preserve SPA shell headers")
	}
}

func TestSPAPathValuesAreNotInterpolated(t *testing.T) {
	handler := spaHandler(t)
	index := serve(handler, http.MethodGet, "/")
	response := serve(handler, http.MethodGet, "/databases/%3Cscript%3E/query")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	if response.Body.String() != index.Body.String() {
		t.Fatal("URL input must not be interpolated into the SPA HTML")
	}
}
