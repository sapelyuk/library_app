// HTTP surface of the user service: the grpc-gateway reverse proxy that exposes
// the gRPC API as REST, plus the Swagger UI documenting those endpoints.
//
// The gateway is a proxy, not a second implementation: a REST request is turned
// into a gRPC call against the server of this very process, so the authorisation
// interceptor and the use case layer run for both transports without a copy of
// any rule.
package handler

import (
	"context"
	_ "embed"
	"fmt"
	"net/http"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/sapelyuk/smart-library/services/user-service/docs"
	userv1 "github.com/sapelyuk/smart-library/services/user-service/gen/go/user/v1"
)

// Addresses of the documentation endpoints. Everything under restPrefix is
// handled by the generated gateway.
const (
	restPrefix    = "/v1/"
	swaggerPrefix = "/swagger/"
	swaggerSpec   = "/swagger/swagger.json"
)

// swaggerUIPage loads Swagger UI from a CDN and points it at the specification
// this service serves itself, so no static assets are needed on disk.
//
//go:embed swagger.html
var swaggerUIPage []byte

// NewREST wires the REST endpoints of the service to the gRPC API. The client is
// expected to talk to the gRPC server of this very process.
func NewREST(client userv1.UserServiceClient) (http.Handler, error) {
	gateway := runtime.NewServeMux(
		runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{
			// Keep zero values in the payloads: an empty phone is easier to read
			// than an absent key while the API is explored in a browser.
			MarshalOptions: protojson.MarshalOptions{EmitUnpopulated: true},
		}),
	)

	if err := userv1.RegisterUserServiceHandlerClient(context.Background(), gateway, client); err != nil {
		return nil, fmt.Errorf("register user service gateway: %w", err)
	}

	mux := http.NewServeMux()
	mux.Handle(restPrefix, gateway)
	mux.HandleFunc(swaggerSpec, serveSwaggerSpec)
	mux.HandleFunc(swaggerPrefix, serveSwaggerUI)
	mux.Handle("/", http.RedirectHandler(swaggerPrefix, http.StatusFound))

	return mux, nil
}

// serveSwaggerSpec answers with the specification generated from user.proto.
func serveSwaggerSpec(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")

	_, _ = w.Write(docs.SwaggerJSON)
}

// serveSwaggerUI renders the documentation shell for every path below /swagger/.
func serveSwaggerUI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")

	_, _ = w.Write(swaggerUIPage)
}
