package handler

import (
	"context"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"

	userv1 "github.com/sapelyuk/smart-library/services/user-service/gen/go/user/v1"
	"github.com/sapelyuk/smart-library/services/user-service/internal/domain"
	"github.com/sapelyuk/smart-library/services/user-service/internal/service"
)

// Register creates a reader account.
//
// The role is not part of the request on purpose: whoever registers becomes a
// reader, and only a librarian can raise that afterwards. Signing in is a separate
// call, so the response carries the account and never a credential.
func (h *Handler) Register(ctx context.Context, req *userv1.RegisterRequest) (*userv1.User, error) {
	user, err := h.service.Register(ctx, service.RegisterInput{
		Email:    req.GetEmail(),
		Password: req.GetPassword(),
		FullName: req.GetFullName(),
		Phone:    req.GetPhone(),
	})
	if err != nil {
		return nil, h.toGRPC(ctx, err)
	}

	h.log.InfoContext(ctx, "account registered", "user_id", user.ID)

	return protoUser(user), nil
}

// Login checks the credentials and issues a bearer token.
func (h *Handler) Login(ctx context.Context, req *userv1.LoginRequest) (*userv1.Session, error) {
	credentials, err := h.service.Login(ctx, req.GetEmail(), req.GetPassword())
	if err != nil {
		return nil, h.toGRPC(ctx, err)
	}

	return protoCredentials(credentials), nil
}

// Logout revokes every session of the caller.
func (h *Handler) Logout(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	caller, err := principalOf(ctx)
	if err != nil {
		return nil, h.toGRPC(ctx, err)
	}

	if err := h.service.Logout(ctx, caller); err != nil {
		return nil, h.toGRPC(ctx, err)
	}

	return &emptypb.Empty{}, nil
}

// GetCurrentUser answers "who am I" for the client that holds the token.
func (h *Handler) GetCurrentUser(ctx context.Context, _ *emptypb.Empty) (*userv1.User, error) {
	caller, err := principalOf(ctx)
	if err != nil {
		return nil, h.toGRPC(ctx, err)
	}

	user, err := h.service.GetCurrentUser(ctx, caller)
	if err != nil {
		return nil, h.toGRPC(ctx, err)
	}

	return protoUser(user), nil
}

// AuthenticateToken resolves a bearer token into the principal behind it.
//
// This is the call the other services of the platform make, and the reason the
// token travels as a request field instead of metadata: the caller of this RPC
// forwards somebody else's token, not its own credentials. It is listed as an
// internal method of the contract and has no REST route.
func (h *Handler) AuthenticateToken(ctx context.Context, req *userv1.AuthenticateTokenRequest) (*userv1.Principal, error) {
	principal, expiresAt, err := h.authenticate(ctx, req.GetToken())
	if err != nil {
		return nil, h.toGRPC(ctx, err)
	}

	return protoPrincipal(principal, expiresAt), nil
}

// authenticate is the single place a token becomes a principal. The interceptor
// and AuthenticateToken both come through here, so the two can never disagree
// about what a valid token is.
func (h *Handler) authenticate(ctx context.Context, token string) (domain.Principal, time.Time, error) {
	principal, session, err := h.service.AuthenticateSession(ctx, token)
	if err != nil {
		return domain.Principal{}, time.Time{}, err
	}

	// The expiry travels with the principal so a caller that forwards the
	// identity can also forward how long it stays valid.
	return principal, session.ExpiresAt, nil
}
