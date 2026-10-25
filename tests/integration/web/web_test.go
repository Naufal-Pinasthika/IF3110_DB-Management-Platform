package web_test

import (
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/web"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPagesRender(t *testing.T) {
	handler, err := web.New()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		"/login", "/databases", "/databases/db-1", "/databases/db-1/query",
		"/databases/db-1/schemas/public/tables/users", "/databases/db-1/migrations",
		"/databases/db-1/migrations/new", "/databases/db-1/migrations/m-1",
		"/admin/users", "/admin/groups", "/admin/databases",
	} {
		t.Run(path, func(t *testing.T) {
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
			if w.Code != 200 {
				t.Fatalf("status = %d: %s", w.Code, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), `/static/app.css`) || !strings.Contains(w.Body.String(), `/static/app.js`) {
				t.Fatal("missing separated assets")
			}
			if !strings.Contains(w.Header().Get("Content-Type"), "text/html") {
				t.Fatal("page is not HTML")
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("page must not be cached")
			}
		})
	}
}

func TestRouteBoundaries(t *testing.T) {
	handler, err := web.New()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		method, path string
		want         int
	}{
		{"GET", "/", 303}, {"GET", "/missing", 404}, {"POST", "/login", 405},
		{"GET", "/static/app.css", 200}, {"GET", "/static/app.js", 200},
		{"GET", "/static/missing", 404},
	} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		if w.Code != tc.want {
			t.Errorf("%s %s = %d, want %d", tc.method, tc.path, w.Code, tc.want)
		}
	}
}

func TestTemplateEscapesPathValues(t *testing.T) {
	handler, err := web.New()
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/databases/%3Cscript%3E/query", nil))
	if w.Code != 200 {
		t.Fatalf("status = %d", w.Code)
	}
	if strings.Contains(w.Body.String(), `<script>`) {
		t.Fatal("path value was rendered as executable HTML")
	}
	if !strings.Contains(w.Body.String(), `&lt;script&gt;`) {
		t.Fatal("missing escaped path data")
	}
}
