package handler

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/sapelyuk/smart-library/services/user-service/internal/domain"
)

// Keys of the request scoped values this package puts on the context.
type contextKey int

const principalKey contextKey = iota

// mdAuthorization is the metadata key gRPC lowercases every header into. The
// grpc-gateway forwards the Authorization header of a REST call under exactly this
// key, which is why one interceptor serves both transports.
const mdAuthorization = "authorization"

// bearerPrefix is the scheme the contract documents. The comparison is
// case insensitive because the HTTP spec defines the scheme that way.
const bearerPrefix = "bearer "

// publicMethods are the RPCs that run without a principal.
//
// Register and Login are how a caller obtains a token, so they cannot require
// one. AuthenticateToken carries the token as a request field and validates it
// itself: the service that calls it forwards somebody else's credentials, and
// making it present its own would confuse the identity of the request with the
// identity under test. Health is served by the standard health service, which has
// its own server and never reaches this interceptor.
var publicMethods = map[string]struct{}{
	"/user.v1.UserService/Register":          {},
	"/user.v1.UserService/Login":             {},
	"/user.v1.UserService/AuthenticateToken": {},
}

// Authenticator resolves a bearer token. *service.Service is the implementation;
// the interface exists so the interceptor can be tested without a database.
type Authenticator interface {
	Authenticate(ctx context.Context, token string) (domain.Principal, error)
}

// NewAuthUnaryInterceptor returns the interceptor that turns the bearer token of a
// call into the principal every use case demands.
//
// The token is resolved once, at the edge, and put on the context; the handlers
// read it from there. Doing it per handler instead would mean twelve copies of the
// same check, and the history of this pattern is that the copy that gets forgotten
// is the one that matters.
func NewAuthUnaryInterceptor(auth Authenticator) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		if _, isPublic := publicMethods[info.FullMethod]; isPublic {
			return handler(ctx, req)
		}

		token, ok := bearerToken(ctx)
		if !ok {
			return nil, toStatus(domain.ErrUnauthenticated)
		}

		principal, err := auth.Authenticate(ctx, token)
		if err != nil {
			return nil, toStatus(err)
		}

		return handler(withPrincipal(ctx, principal), req)
	}
}

// toStatus translates an error of the authentication path into a status error.
//
// The interceptor runs before any handler, so it cannot leave the translation to
// the handler: an error it returns directly would reach the client as an
// "unknown" status, which the REST gateway renders as 500. A missing token has to
// be a 401, not a server fault.
func toStatus(err error) error {
	if st, ok := status.FromError(err); ok && st.Code() != codes.Unknown {
		return st.Err()
	}

	if code, mapped := codeFor(err); mapped {
		return status.Error(code, err.Error())
	}

	return status.Error(codes.Internal, "internal error")
}

// bearerToken pulls the credential out of the metadata of the call.
func bearerToken(ctx context.Context) (string, bool) {
	values, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", false
	}

	// A request that carries the header twice is not a request this service
	// guesses about: which of the two tokens would be the caller?
	raw := values.Get(mdAuthorization)
	if len(raw) != 1 {
		return "", false
	}

	value := strings.TrimSpace(raw[0])
	if len(value) < len(bearerPrefix) || !strings.EqualFold(value[:len(bearerPrefix)], bearerPrefix) {
		return "", false
	}

	token := strings.TrimSpace(value[len(bearerPrefix):])

	return token, token != ""
}

// withPrincipal puts the resolved caller on the context of the call.
func withPrincipal(ctx context.Context, principal domain.Principal) context.Context {
	return context.WithValue(ctx, principalKey, principal)
}

// principalOf reads the caller back. A missing principal is
// domain.ErrUnauthenticated rather than a panic: an RPC that was added to the
// service without being classified in publicMethods must fail closed.
func principalOf(ctx context.Context) (domain.Principal, error) {
	principal, ok := ctx.Value(principalKey).(domain.Principal)
	if !ok || principal.UserID == uuid.Nil {
		return domain.Principal{}, domain.ErrUnauthenticated
	}

	return principal, nil
}
