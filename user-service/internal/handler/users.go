package handler

import (
	"context"

	"google.golang.org/protobuf/types/known/emptypb"

	userv1 "library_app/user-service/gen/go/user/v1"
	"library_app/user-service/internal/domain"
	"library_app/user-service/internal/service"
)

// CreateUser creates an account of a chosen role, librarians only.
//
// The public way into the system is Register; this is the way a librarian adds a
// colleague or fixes up a reader who cannot self register.
func (h *Handler) CreateUser(ctx context.Context, req *userv1.CreateUserRequest) (*userv1.User, error) {
	caller, err := principalOf(ctx)
	if err != nil {
		return nil, h.toGRPC(ctx, err)
	}

	role, err := roleFromProto(req.GetRole())
	if err != nil {
		return nil, h.toGRPC(ctx, err)
	}

	user, err := h.service.CreateUser(ctx, caller, service.CreateUserInput{
		Email:    req.GetEmail(),
		Password: req.GetPassword(),
		FullName: req.GetFullName(),
		Phone:    req.GetPhone(),
		Role:     role,
	})
	if err != nil {
		return nil, h.toGRPC(ctx, err)
	}

	return protoUser(user), nil
}

// GetUser returns one account by its identifier.
func (h *Handler) GetUser(ctx context.Context, req *userv1.GetUserRequest) (*userv1.User, error) {
	caller, err := principalOf(ctx)
	if err != nil {
		return nil, h.toGRPC(ctx, err)
	}

	id, err := parseID(req.GetId())
	if err != nil {
		return nil, h.toGRPC(ctx, err)
	}

	user, err := h.service.GetUser(ctx, caller, id)
	if err != nil {
		return nil, h.toGRPC(ctx, err)
	}

	return protoUser(user), nil
}

// ListUsers returns one page of the accounts, librarians only.
func (h *Handler) ListUsers(ctx context.Context, req *userv1.ListUsersRequest) (*userv1.ListUsersResponse, error) {
	caller, err := principalOf(ctx)
	if err != nil {
		return nil, h.toGRPC(ctx, err)
	}

	filter, err := userFilter(req)
	if err != nil {
		return nil, h.toGRPC(ctx, err)
	}

	users, total, err := h.service.ListUsers(ctx, caller, filter)
	if err != nil {
		return nil, h.toGRPC(ctx, err)
	}

	return &userv1.ListUsersResponse{
		Users: protoUsers(users),
		Total: int32(total),
	}, nil
}

// UpdateUser changes the profile of an account.
//
// The optional fields of the request are the field mask: a field that was not set
// stays as it is, which is what makes the endpoint safe for a form that only shows
// part of the profile.
func (h *Handler) UpdateUser(ctx context.Context, req *userv1.UpdateUserRequest) (*userv1.User, error) {
	caller, err := principalOf(ctx)
	if err != nil {
		return nil, h.toGRPC(ctx, err)
	}

	id, err := parseID(req.GetId())
	if err != nil {
		return nil, h.toGRPC(ctx, err)
	}

	update, err := userUpdate(req)
	if err != nil {
		return nil, h.toGRPC(ctx, err)
	}

	user, err := h.service.UpdateUser(ctx, caller, id, update)
	if err != nil {
		return nil, h.toGRPC(ctx, err)
	}

	return protoUser(user), nil
}

// ChangePassword replaces the password of an account.
//
// The presence of current_password decides who is calling: the owner proves the old
// password, a librarian who resets an account does not have it and says so by
// leaving the field out. The rules of the two paths differ, so the service is told
// which one this is rather than left to guess from an empty string.
func (h *Handler) ChangePassword(ctx context.Context, req *userv1.ChangePasswordRequest) (*emptypb.Empty, error) {
	caller, err := principalOf(ctx)
	if err != nil {
		return nil, h.toGRPC(ctx, err)
	}

	id, err := parseID(req.GetId())
	if err != nil {
		return nil, h.toGRPC(ctx, err)
	}

	byOwner := req.GetCurrentPassword() != ""

	if _, err := h.service.ChangePassword(
		ctx, caller, id, req.GetCurrentPassword(), req.GetNewPassword(), byOwner,
	); err != nil {
		return nil, h.toGRPC(ctx, err)
	}

	return &emptypb.Empty{}, nil
}

// DeactivateUser blocks an account and revokes its sessions.
func (h *Handler) DeactivateUser(ctx context.Context, req *userv1.DeactivateUserRequest) (*userv1.User, error) {
	caller, err := principalOf(ctx)
	if err != nil {
		return nil, h.toGRPC(ctx, err)
	}

	id, err := parseID(req.GetId())
	if err != nil {
		return nil, h.toGRPC(ctx, err)
	}

	user, err := h.service.DeactivateUser(ctx, caller, id)
	if err != nil {
		return nil, h.toGRPC(ctx, err)
	}

	return protoUser(user), nil
}

// RestoreUser unblocks an account.
func (h *Handler) RestoreUser(ctx context.Context, req *userv1.RestoreUserRequest) (*userv1.User, error) {
	caller, err := principalOf(ctx)
	if err != nil {
		return nil, h.toGRPC(ctx, err)
	}

	id, err := parseID(req.GetId())
	if err != nil {
		return nil, h.toGRPC(ctx, err)
	}

	user, err := h.service.RestoreUser(ctx, caller, id)
	if err != nil {
		return nil, h.toGRPC(ctx, err)
	}

	return protoUser(user), nil
}

// userFilter converts the listing request into the domain filter. An unspecified
// enum means "no constraint on that field", which is the empty domain value.
func userFilter(req *userv1.ListUsersRequest) (domain.UserFilter, error) {
	filter := domain.UserFilter{
		Query:  req.GetQuery(),
		Limit:  int(req.GetLimit()),
		Offset: int(req.GetOffset()),
	}

	if role := req.GetRole(); role != userv1.Role_ROLE_UNSPECIFIED {
		converted, err := roleFromProto(role)
		if err != nil {
			return domain.UserFilter{}, err
		}

		filter.Role = converted
	}

	if status := req.GetStatus(); status != userv1.UserStatus_USER_STATUS_UNSPECIFIED {
		converted, err := statusFromProto(status)
		if err != nil {
			return domain.UserFilter{}, err
		}

		filter.Status = converted
	}

	return filter, nil
}

// userUpdate converts the update request into the domain update, keeping the
// distinction between "set to empty" and "not sent at all".
func userUpdate(req *userv1.UpdateUserRequest) (domain.UserUpdate, error) {
	update := domain.UserUpdate{}

	if req.FullName != nil {
		value := req.GetFullName()
		update.FullName = &value
	}

	if req.Phone != nil {
		value := req.GetPhone()
		update.Phone = &value
	}

	if req.Email != nil {
		value := req.GetEmail()
		update.Email = &value
	}

	if req.Role != nil {
		role, err := roleFromProto(req.GetRole())
		if err != nil {
			return domain.UserUpdate{}, err
		}

		update.Role = &role
	}

	if req.Status != nil {
		status, err := statusFromProto(req.GetStatus())
		if err != nil {
			return domain.UserUpdate{}, err
		}

		update.Status = &status
	}

	return update, nil
}
