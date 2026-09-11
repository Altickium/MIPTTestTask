package httpapi

import (
	"embed"
	"html/template"
	"net/http"
	"strings"

	apispec "pollservice/api"
)

//go:embed web/templates/*.html web/static/*
var embeddedWeb embed.FS
var adminTemplate = template.Must(template.ParseFS(embeddedWeb, "web/templates/admin.html"))

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' blob: data:; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

func (a *API) home(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/admin", http.StatusFound)
}
func (a *API) pollPage(w http.ResponseWriter, _ *http.Request) {
	serveEmbedded(w, "web/templates/poll.html", "text/html; charset=utf-8", "no-store")
}
func (a *API) adminPage(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := adminTemplate.ExecuteTemplate(w, "admin.html", struct{ PublicBaseURL string }{a.publicBaseURL}); err != nil {
		a.log.Error("admin template failed", "error", err)
	}
}
func (a *API) staticAsset(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	contentType := ""
	switch {
	case name == "app.css":
		contentType = "text/css; charset=utf-8"
	case strings.HasSuffix(name, ".js") && (name == "poll.js" || name == "admin.js"):
		contentType = "text/javascript; charset=utf-8"
	default:
		http.NotFound(w, r)
		return
	}
	serveEmbedded(w, "web/static/"+name, contentType, "public, max-age=31536000, immutable")
}
func (a *API) openAPISpec(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(apispec.OpenAPI)
}
func serveEmbedded(w http.ResponseWriter, name, contentType, cacheControl string) {
	data, err := embeddedWeb.ReadFile(name)
	if err != nil {
		http.Error(w, "asset unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", cacheControl)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
