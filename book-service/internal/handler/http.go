// Package handler adapts transports to the service layer. This file builds the
// HTTP surface of the service: the grpc-gateway reverse proxy that exposes the
// gRPC API as REST, plus the Swagger UI documenting those endpoints.
package handler

import (
	"context"
	_ "embed"
	"fmt"
	"net/http"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/protobuf/encoding/protojson"

	"library_app/book-service/docs"
	bookv1 "library_app/book-service/gen/go/book/v1"
)

// Addresses of the documentation endpoints. Everything under restPrefix is
// handled by the generated gateway.
const (
	restPrefix    = "/v1/"
	swaggerPrefix = "/swagger/"
	swaggerSpec   = "/swagger/swagger.json"
)

// swaggerUIPage loads Swagger UI from a CDN and points it at the embedded
// specification, so the service needs no static assets on disk.
//
//go:embed swagger.html
var swaggerUIPage []byte

// NewREST wires the REST endpoints of the service to the gRPC API. The client
// is expected to talk to the gRPC server of this very process.
func NewREST(client bookv1.BookServiceClient) (http.Handler, error) {
	gateway := runtime.NewServeMux(
		runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{
			// Keep zero values in the payloads: empty counters and empty strings
			// are easier to read while exploring the API in a browser.
			MarshalOptions: protojson.MarshalOptions{EmitUnpopulated: true},
		}),
	)

	if err := bookv1.RegisterBookServiceHandlerClient(context.Background(), gateway, client); err != nil {
		return nil, fmt.Errorf("register book service gateway: %w", err)
	}

	mux := http.NewServeMux()
	mux.Handle(restPrefix, gateway)
	mux.HandleFunc(swaggerSpec, serveSwaggerSpec)
	mux.HandleFunc(swaggerPrefix, serveSwaggerUI)
	mux.Handle("/", http.RedirectHandler(swaggerPrefix, http.StatusFound))

	return mux, nil
}

// serveSwaggerSpec answers with the specification generated from book.proto.
func serveSwaggerSpec(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")

	_, _ = w.Write(docs.SwaggerJSON)
}

// serveSwaggerUI renders the Swagger UI shell for every path below /swagger/.
func serveSwaggerUI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")

	_, _ = w.Write(swaggerUIPage)
}
