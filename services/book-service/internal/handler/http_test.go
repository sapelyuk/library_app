package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/grpc"

	bookv1 "github.com/sapelyuk/smart-library/services/book-service/gen/go/book/v1"
	"github.com/sapelyuk/smart-library/services/book-service/internal/handler"
)

// stubClient records the request the gateway built and answers with a canned
// book; the remaining methods come from the embedded nil interface.
type stubClient struct {
	bookv1.BookServiceClient

	createIn *bookv1.CreateBookRequest
}

func (s *stubClient) CreateBook(_ context.Context, in *bookv1.CreateBookRequest, _ ...grpc.CallOption) (*bookv1.Book, error) {
	s.createIn = in

	return &bookv1.Book{Id: "11111111-1111-1111-1111-111111111111", Title: in.GetTitle(), TotalCopies: 0}, nil
}

func newServer(t *testing.T, client bookv1.BookServiceClient) *httptest.Server {
	t.Helper()

	httpHandler, err := handler.NewREST(client)
	if err != nil {
		t.Fatalf("build rest handler: %v", err)
	}

	server := httptest.NewServer(httpHandler)
	t.Cleanup(server.Close)

	return server
}

func TestCreateBookRoute(t *testing.T) {
	client := &stubClient{}
	server := newServer(t, client)

	body := `{"isbn":"978-0-13-419044-0","title":"The Go Programming Language","author":"Donovan","publisher":"Addison-Wesley","published_year":2015}`

	resp, err := http.Post(server.URL+"/v1/books", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("post /v1/books: %v", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var decoded map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if got := decoded["id"]; got != "11111111-1111-1111-1111-111111111111" {
		t.Errorf("id = %v, want the id returned by the service", got)
	}

	// Zero values must stay in the payload so the browser sees every counter.
	if _, ok := decoded["totalCopies"]; !ok {
		t.Error("totalCopies is missing from the response, zero values are not emitted")
	}

	if client.createIn.GetPublishedYear() != 2015 {
		t.Errorf("published_year = %d, want 2015: the body was not mapped onto the request", client.createIn.GetPublishedYear())
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

	for _, path := range []string{"/v1/books", "/v1/books/{id}", "/v1/books/{bookId}/borrow", "/v1/copies/{copyId}/return"} {
		if _, ok := paths[path]; !ok {
			t.Errorf("swagger.json is missing path %s", path)
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
