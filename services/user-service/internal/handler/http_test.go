package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	userv1 "github.com/sapelyuk/smart-library/services/user-service/gen/go/user/v1"
	"github.com/sapelyuk/smart-library/services/user-service/internal/handler"
)

// stubClient records the requests the gateway built and answers with canned
// values; the remaining methods come from the embedded nil interface.
type stubClient struct {
	userv1.UserServiceClient

	registerIn *userv1.RegisterRequest
	loginIn    *userv1.LoginRequest
	getUserIn  *userv1.GetUserRequest
	listIn     *userv1.ListUsersRequest
	updateIn   *userv1.UpdateUserRequest
	passwordIn *userv1.ChangePasswordRequest

	// loginErr, when set, is returned by Login to exercise the error mapping.
	loginErr error
}

func (s *stubClient) Register(_ context.Context, in *userv1.RegisterRequest, _ ...grpc.CallOption) (*userv1.User, error) {
	s.registerIn = in

	return &userv1.User{
		Id:       "22222222-2222-2222-2222-222222222222",
		Email:    in.GetEmail(),
		FullName: in.GetFullName(),
		Role:     userv1.Role_ROLE_READER,
		Status:   userv1.UserStatus_USER_STATUS_ACTIVE,
	}, nil
}

func (s *stubClient) Login(_ context.Context, in *userv1.LoginRequest, _ ...grpc.CallOption) (*userv1.Session, error) {
	s.loginIn = in

	if s.loginErr != nil {
		return nil, s.loginErr
	}

	return &userv1.Session{
		Id:          "33333333-3333-3333-3333-333333333333",
		UserId:      "22222222-2222-2222-2222-222222222222",
		AccessToken: "opaque-token",
	}, nil
}

func (s *stubClient) GetUser(_ context.Context, in *userv1.GetUserRequest, _ ...grpc.CallOption) (*userv1.User, error) {
	s.getUserIn = in

	return &userv1.User{
		Id:       in.GetId(),
		Email:    "reader@example.com",
		FullName: "Anna Reader",
		Role:     userv1.Role_ROLE_READER,
		Status:   userv1.UserStatus_USER_STATUS_ACTIVE,
	}, nil
}

func (s *stubClient) ListUsers(_ context.Context, in *userv1.ListUsersRequest, _ ...grpc.CallOption) (*userv1.ListUsersResponse, error) {
	s.listIn = in

	return &userv1.ListUsersResponse{
		Users: []*userv1.User{{
			Id:    "22222222-2222-2222-2222-222222222222",
			Email: "reader@example.com",
			Role:  userv1.Role_ROLE_READER,
		}},
		Total: 1,
	}, nil
}

func (s *stubClient) UpdateUser(_ context.Context, in *userv1.UpdateUserRequest, _ ...grpc.CallOption) (*userv1.User, error) {
	s.updateIn = in

	return &userv1.User{
		Id:       in.GetId(),
		FullName: in.GetFullName(),
		Phone:    in.GetPhone(),
	}, nil
}

func (s *stubClient) ChangePassword(_ context.Context, in *userv1.ChangePasswordRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	s.passwordIn = in

	return &emptypb.Empty{}, nil
}

func newServer(t *testing.T, client userv1.UserServiceClient) *httptest.Server {
	t.Helper()

	httpHandler, err := handler.NewREST(client)
	if err != nil {
		t.Fatalf("build rest handler: %v", err)
	}

	server := httptest.NewServer(httpHandler)
	t.Cleanup(server.Close)

	return server
}

func TestRegisterRoute(t *testing.T) {
	client := &stubClient{}
	server := newServer(t, client)

	body := `{"email":"reader@example.com","password":"Str0ng-Pass!","full_name":"Anna Reader","phone":"+79001234567"}`

	resp, err := http.Post(server.URL+"/v1/auth/register", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("post /v1/auth/register: %v", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var decoded map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if got := decoded["id"]; got != "22222222-2222-2222-2222-222222222222" {
		t.Errorf("id = %v, want the id returned by the service", got)
	}

	// snake_case field names from the proto contract must survive the mapping.
	if got := decoded["fullName"]; got != "Anna Reader" {
		t.Errorf("fullName = %v, want %q", got, "Anna Reader")
	}

	// Zero values must stay in the payload so the browser sees every field.
	if _, ok := decoded["lastLoginAt"]; !ok {
		t.Error("lastLoginAt is missing from the response, zero values are not emitted")
	}

	if client.registerIn.GetEmail() != "reader@example.com" {
		t.Errorf("email = %q, want %q: the body was not mapped onto the request", client.registerIn.GetEmail(), "reader@example.com")
	}

	if client.registerIn.GetFullName() != "Anna Reader" {
		t.Errorf("full_name = %q, want %q: snake_case body field was not mapped", client.registerIn.GetFullName(), "Anna Reader")
	}
}

func TestLoginRoute(t *testing.T) {
	client := &stubClient{}
	server := newServer(t, client)

	body := `{"email":"reader@example.com","password":"Str0ng-Pass!"}`

	resp, err := http.Post(server.URL+"/v1/auth/login", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("post /v1/auth/login: %v", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var decoded map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	// The token is the whole point of the endpoint: it must reach the client
	// under the camelCase name the Swagger UI documents.
	if got := decoded["accessToken"]; got != "opaque-token" {
		t.Errorf("accessToken = %v, want the token returned by the service", got)
	}

	if client.loginIn.GetPassword() != "Str0ng-Pass!" {
		t.Errorf("password = %q, want it mapped from the body", client.loginIn.GetPassword())
	}
}

func TestLoginAuthErrorMapping(t *testing.T) {
	client := &stubClient{loginErr: status.Error(codes.Unauthenticated, "invalid credentials")}
	server := newServer(t, client)

	body := `{"email":"reader@example.com","password":"wrong"}`

	resp, err := http.Post(server.URL+"/v1/auth/login", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("post /v1/auth/login: %v", err)
	}

	defer resp.Body.Close()

	// The gateway must translate the gRPC code, not answer 200 with an error
	// payload: UNAUTHENTICATED is HTTP 401.
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}

	var decoded map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode error response: %v", err)
	}

	if got := decoded["code"]; got != float64(codes.Unauthenticated) {
		t.Errorf("error code = %v, want %d (UNAUTHENTICATED)", got, codes.Unauthenticated)
	}
}

func TestGetUserRoute(t *testing.T) {
	client := &stubClient{}
	server := newServer(t, client)

	resp, err := http.Get(server.URL + "/v1/users/22222222-2222-2222-2222-222222222222")
	if err != nil {
		t.Fatalf("get /v1/users/{id}: %v", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// The path segment must land in the request message.
	if client.getUserIn.GetId() != "22222222-2222-2222-2222-222222222222" {
		t.Errorf("id = %q, want the id from the path", client.getUserIn.GetId())
	}

	var decoded map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if got := decoded["fullName"]; got != "Anna Reader" {
		t.Errorf("fullName = %v, want %q", got, "Anna Reader")
	}
}

func TestListUsersQueryParameters(t *testing.T) {
	client := &stubClient{}
	server := newServer(t, client)

	resp, err := http.Get(server.URL + "/v1/users?query=anna&role=ROLE_READER&limit=5&offset=10")
	if err != nil {
		t.Fatalf("get /v1/users: %v", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	if client.listIn.GetQuery() != "anna" {
		t.Errorf("query = %q, want %q: the query string was not mapped", client.listIn.GetQuery(), "anna")
	}

	if client.listIn.GetRole() != userv1.Role_ROLE_READER {
		t.Errorf("role = %v, want ROLE_READER", client.listIn.GetRole())
	}

	if client.listIn.GetLimit() != 5 || client.listIn.GetOffset() != 10 {
		t.Errorf("limit/offset = %d/%d, want 5/10", client.listIn.GetLimit(), client.listIn.GetOffset())
	}

	var decoded map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if got := decoded["total"]; got != float64(1) {
		t.Errorf("total = %v, want 1", got)
	}
}

func TestUpdateUserRoute(t *testing.T) {
	client := &stubClient{}
	server := newServer(t, client)

	req, err := http.NewRequest(http.MethodPatch, server.URL+"/v1/users/22222222-2222-2222-2222-222222222222",
		strings.NewReader(`{"fullName":"Anna R.","phone":"+79007654321"}`))
	if err != nil {
		t.Fatalf("build patch request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("patch /v1/users/{id}: %v", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	if client.updateIn.GetId() != "22222222-2222-2222-2222-222222222222" {
		t.Errorf("id = %q, want the id from the path", client.updateIn.GetId())
	}

	// optional fields: the presence of the key must reach the service as a
	// set field, distinguishable from an absent one.
	if client.updateIn.GetFullName() != "Anna R." {
		t.Errorf("full_name = %q, want %q", client.updateIn.GetFullName(), "Anna R.")
	}

	if client.updateIn.Phone == nil {
		t.Error("phone is nil: the optional field was lost in the mapping")
	}
}

func TestChangePasswordRoute(t *testing.T) {
	client := &stubClient{}
	server := newServer(t, client)

	body := `{"currentPassword":"Old-Pass!","newPassword":"New-Pass!"}`

	resp, err := http.Post(server.URL+"/v1/users/22222222-2222-2222-2222-222222222222/password", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("post /v1/users/{id}/password: %v", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	if client.passwordIn.GetId() != "22222222-2222-2222-2222-222222222222" {
		t.Errorf("id = %q, want the id from the path", client.passwordIn.GetId())
	}

	if client.passwordIn.GetNewPassword() != "New-Pass!" {
		t.Errorf("new_password = %q, want it mapped from the body", client.passwordIn.GetNewPassword())
	}
}

func TestSwaggerEndpoints(t *testing.T) {
	server := newServer(t, &stubClient{})

	specResp, err := http.Get(server.URL + "/swagger/swagger.json")
	if err != nil {
		t.Fatalf("get swagger.json: %v", err)
	}

	defer specResp.Body.Close()

	if specResp.StatusCode != http.StatusOK {
		t.Fatalf("swagger.json status = %d, want %d", specResp.StatusCode, http.StatusOK)
	}

	if got := specResp.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Errorf("swagger.json content type = %q, want application/json", got)
	}

	var spec map[string]any
	if err := json.NewDecoder(specResp.Body).Decode(&spec); err != nil {
		t.Fatalf("decode swagger.json: %v", err)
	}

	paths, ok := spec["paths"].(map[string]any)
	if !ok {
		t.Fatal("swagger.json has no paths section")
	}

	for _, path := range []string{
		"/v1/auth/register",
		"/v1/auth/login",
		"/v1/auth/logout",
		"/v1/auth/me",
		"/v1/users",
		"/v1/users/{id}",
		"/v1/users/{id}/password",
		"/v1/users/{id}/deactivate",
		"/v1/users/{id}/restore",
	} {
		if _, ok := paths[path]; !ok {
			t.Errorf("swagger.json is missing path %s", path)
		}
	}

	// AuthenticateToken is an internal method: it must stay out of the public
	// REST surface documented by Swagger.
	for path := range paths {
		if strings.Contains(path, "authenticate") || strings.Contains(path, "token") {
			t.Errorf("swagger.json exposes the internal AuthenticateToken route %s", path)
		}
	}

	uiResp, err := http.Get(server.URL + "/swagger/")
	if err != nil {
		t.Fatalf("get /swagger/: %v", err)
	}

	defer uiResp.Body.Close()

	if uiResp.StatusCode != http.StatusOK {
		t.Fatalf("/swagger/ status = %d, want %d", uiResp.StatusCode, http.StatusOK)
	}

	if got := uiResp.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Errorf("/swagger/ content type = %q, want text/html", got)
	}

	noRedirect := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	rootResp, err := noRedirect.Get(server.URL + "/")
	if err != nil {
		t.Fatalf("get /: %v", err)
	}

	defer rootResp.Body.Close()

	if rootResp.StatusCode != http.StatusFound || rootResp.Header.Get("Location") != "/swagger/" {
		t.Errorf("/ = %d %q, want 302 -> /swagger/", rootResp.StatusCode, rootResp.Header.Get("Location"))
	}
}
