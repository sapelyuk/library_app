package security

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// TokenBytes is the entropy of a session token: 256 bits, which is also the
// size of the SHA-256 digest stored next to it.
const TokenBytes = 32

// NewToken returns an opaque bearer token.
//
// The token is a random string rather than a signed claim set on purpose: a
// session has to be revocable the moment a librarian blocks an account, and a
// self contained JWT cannot promise that without a second lookup anyway. The
// value is base64url without padding so it survives a header verbatim.
func NewToken() (string, error) {
	raw := make([]byte, TokenBytes)

	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("security: read token: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// HashToken returns the hex encoded SHA-256 digest of a token, the only form in
// which a token may be persisted. Hashing before storing means a leaked table
// does not hand out live credentials, and the lookup stays a plain unique index
// probe instead of a scan over candidate tokens.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))

	return hex.EncodeToString(sum[:])
}
