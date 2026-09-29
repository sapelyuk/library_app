// Package docs ships the OpenAPI document of the REST API and a documentation
// page, both embedded in the binary so a deployed instance describes itself.
package docs

import (
	"bytes"
	"embed"
	"net/http"
)

//go:embed openapi.yaml swagger.html
var assets embed.FS

// OpenAPI returns the specification document.
func OpenAPI() []byte {
	raw, err := assets.ReadFile("openapi.yaml")
	if err != nil {
		return []byte("{}")
	}

	return raw
}

// Handler serves the documentation page with the specification injected, so the
// page does not need a second request that would be blocked by CORS.
func Handler() http.Handler {
	spec := OpenAPI()
	page, err := assets.ReadFile("swagger.html")
	if err != nil {
		return http.NotFoundHandler()
	}

	rendered := bytes.ReplaceAll(page, []byte("/*__OPENAPI_SPEC__*/"), spec)

	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(rendered)
	})
}
