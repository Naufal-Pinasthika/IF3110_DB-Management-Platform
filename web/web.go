// Package web serves non-SPA Go templates and embedded static assets.
package web

import (
	"bytes"
	"embed"
	"html/template"
	"net/http"
	"net/url"
)

//go:embed templates/*.html static/*
var files embed.FS

type page struct {
	Kind, Title, Description               string
	DatabaseID, Schema, Table, MigrationID string
	DatabaseURL                            string
}

func New() (http.Handler, error) {
	templates, err := template.ParseFS(files, "templates/*.html")
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/databases", http.StatusSeeOther)
	})
	for _, route := range []struct{ pattern, kind, title, description string }{
		{"/login", "login", "Sign in", "Access your database workspace."},
		{"/databases", "databases", "Databases", "Choose a database to explore its schema, run queries, or manage migrations."},
		{"/databases/{dbId}", "database", "Schema browser", "Browse schemas and tables in this database."},
		{"/databases/{dbId}/schemas/{schema}/tables/{table}", "table", "Table", "Inspect columns, constraints, indexes, and table data."},
		{"/databases/{dbId}/query", "query", "SQL query", "Run a read-only query against this database."},
		{"/databases/{dbId}/migrations", "migrations", "Migrations", "Review schema changes and their execution history."},
		{"/databases/{dbId}/migrations/new", "migration-new", "New migration", "Write and save a draft before executing it."},
		{"/databases/{dbId}/migrations/{migrationId}", "migration", "Migration details", "Review the SQL and execution details."},
		{"/admin/users", "users", "Users", "Manage platform accounts and account access."},
		{"/admin/groups", "groups", "Groups & access", "Manage groups, membership, and database permissions."},
		{"/admin/databases", "connections", "Connections", "Manage PostgreSQL connections available to the platform."},
	} {
		mux.HandleFunc("GET "+route.pattern, func(w http.ResponseWriter, r *http.Request) {
			data := page{Kind: route.kind, Title: route.title, Description: route.description,
				DatabaseID: r.PathValue("dbId"), Schema: r.PathValue("schema"), Table: r.PathValue("table"), MigrationID: r.PathValue("migrationId")}
			if data.Kind == "table" {
				data.Title = data.Schema + "." + data.Table
			}
			if data.DatabaseID != "" {
				data.DatabaseURL = "/databases/" + url.PathEscape(data.DatabaseID)
			}
			var buffer bytes.Buffer
			if err := templates.ExecuteTemplate(&buffer, "page", data); err != nil {
				http.Error(w, "Unable to display this page. Please try again.", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Referrer-Policy", "same-origin")
			w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'; object-src 'none'")
			_, _ = w.Write(buffer.Bytes())
		})
	}

	for _, name := range []string{"app.css", "app.js", "api.mjs", "favicon.ico"} {
		mux.HandleFunc("GET /static/"+name, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			http.ServeFileFS(w, r, files, "static/"+name)
		})
	}
	
	mux.HandleFunc("GET /favicon.ico", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("X-Content-Type-Options", "nosniff")
        http.ServeFileFS(w, r, files, "static/favicon.ico")
    })

	return mux, nil
}
