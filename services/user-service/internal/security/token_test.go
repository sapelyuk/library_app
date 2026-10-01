package security_test

import (
	"encoding/base64"
	"encoding/hex"
	"testing"

	"github.com/sapelyuk/smart-library/services/user-service/internal/security"
)

func TestNewToken(t *testing.T) {
	t.Parallel()

	token, err := security.NewToken()
	if err != nil {
		t.Fatalf("NewToken: unexpected error: %v", err)
	}

	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		t.Fatalf("the token must be base64url without padding: %v", err)
	}

	if len(raw) != security.TokenBytes {
		t.Fatalf("token entropy = %d bytes, want %d", len(raw), security.TokenBytes)
	}
}

func TestNewTokenIsUnique(t *testing.T) {
	t.Parallel()

	const count = 100

	seen := make(map[string]struct{}, count)

	for i := 0; i < count; i++ {
		token, err := security.NewToken()
		if err != nil {
			t.Fatalf("NewToken: unexpected error: %v", err)
		}

		if _, exists := seen[token]; exists {
			t.Fatalf("token %q was generated twice", token)
		}

		seen[token] = struct{}{}
	}
}

func TestHashToken(t *testing.T) {
	t.Parallel()

	// A fixed vector keeps the digest algorithm pinned: switching to anything
	// but SHA-256 would invalidate every stored session.
	const emptySHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

	if got := security.HashToken(""); got != emptySHA256 {
		t.Fatalf("HashToken(\"\") = %q, want %q", got, emptySHA256)
	}

	token, err := security.NewToken()
	if err != nil {
		t.Fatalf("NewToken: unexpected error: %v", err)
	}

	hash := security.HashToken(token)

	if hash != security.HashToken(token) {
		t.Fatal("HashToken must be deterministic")
	}

	raw, err := hex.DecodeString(hash)
	if err != nil {
		t.Fatalf("the digest must be hex encoded: %v", err)
	}

	if len(raw) != 32 {
		t.Fatalf("digest = %d bytes, want 32", len(raw))
	}

	other, err := security.NewToken()
	if err != nil {
		t.Fatalf("NewToken: unexpected error: %v", err)
	}

	if security.HashToken(other) == hash {
		t.Fatal("different tokens must produce different digests")
	}
}
