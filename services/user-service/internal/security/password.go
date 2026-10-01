// Package security owns the credentials of the service: password hashing and
// session tokens. Nothing in this package is safe to move into domain: the
// whole point of the split is that domain never sees a plaintext secret and
// never decides on its own how a secret is verified.
package security

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

// ErrWrongPassword reports a password that does not match the stored hash. It
// is deliberately the only error a caller is allowed to react to: every other
// failure means the stored hash itself is broken and must not be reported as a
// bad password.
var ErrWrongPassword = errors.New("security: wrong password")

// Malformed-hash errors. They are exported as values so that the service can
// answer 500 instead of 401 when the database contains garbage.
var (
	ErrInvalidHash          = errors.New("security: password hash is malformed")
	ErrUnsupportedAlgorithm = errors.New("security: password hash uses an unsupported algorithm")
)

// Parameters are the argon2id settings written into every hash, so an old hash
// keeps verifying after the defaults below are raised.
//
// The defaults follow the OWASP cheat-sheet recommendation for argon2id
// (m=19 MiB, t=2, p=1) and cost roughly 30 ms on a current server core, which
// is the point: a login endpoint must be expensive enough that dumping the
// table does not buy an attacker a usable wordlist.
type Parameters struct {
	// Memory is the argon2 memory parameter in KiB.
	Memory uint32
	// Iterations is the time parameter t.
	Iterations uint32
	// Parallelism is the p parameter, it also fixes the key length factor.
	Parallelism uint8
	// KeyLength is the derived key length in bytes.
	KeyLength uint32
	// SaltLength is the length of the random salt in bytes.
	SaltLength uint32
}

// DefaultParameters is the argon2id profile used for new hashes.
var DefaultParameters = Parameters{
	Memory:      19 * 1024,
	Iterations:  2,
	Parallelism: 1,
	KeyLength:   32,
	SaltLength:  16,
}

// DummyPasswordHash is a valid argon2id encoding of a throwaway string. It is
// verified when the presented account does not exist, so that "unknown email" and
// "wrong password" cost exactly the same CPU and cannot be told apart by timing.
// The value is a constant on purpose: it is nobody's credential, rotating it buys
// nothing.
const DummyPasswordHash = "$argon2id$v=19$m=19456,t=2,p=1$M+WcX/T6DuelhFyZdSWt5w$" +
	"axLRD1iyqoL7kAS7NErFQ+UVSprBhpML5QfVO4mVDHk"

// VerifyDummy runs the verification cost of one real password against
// DummyPasswordHash and throws the answer away. Login calls it when there is no
// such account, which is what keeps the two credential failures equal in time.
func VerifyDummy(password string) {
	_ = VerifyPassword(password, DummyPasswordHash)
}

// String renders the parameters the way they appear in a PHC hash, so a log
// line or a configuration value can be parsed back by ParseParameters.
func (p Parameters) String() string {
	return fmt.Sprintf("argon2id$v=19$m=%d,t=%d,p=%d,k=%d,s=%d",
		p.Memory, p.Iterations, p.Parallelism, p.KeyLength, p.SaltLength)
}

// Validate rejects settings that would be a security downgrade by accident.
func (p Parameters) Validate() error {
	switch {
	case p.Memory < 8*1024:
		return fmt.Errorf("security: argon2 memory %d KiB is below the 8192 KiB floor", p.Memory)
	case p.Iterations < 1:
		return errors.New("security: argon2 iterations must be at least 1")
	case p.Parallelism < 1:
		return errors.New("security: argon2 parallelism must be at least 1")
	case p.KeyLength < 16:
		return fmt.Errorf("security: argon2 key length %d bytes is below the 16 byte floor", p.KeyLength)
	case p.SaltLength < 8:
		return fmt.Errorf("security: argon2 salt length %d bytes is below the 8 byte floor", p.SaltLength)
	}

	return nil
}

// HashPassword returns an encoded argon2id hash of the given password.
//
// The password is NOT pre-hashed with SHA-256: argon2 accepts an arbitrary
// length input, so the usual "hash first to dodge the 72 byte limit" trick of
// bcrypt is unnecessary here and only adds a second, weaker primitive.
func HashPassword(password string) (string, error) {
	return HashPasswordWith(DefaultParameters, password)
}

// HashPasswordWith hashes with explicit parameters, used by the tests and by
// deployments that must pin a profile.
func HashPasswordWith(params Parameters, password string) (string, error) {
	if err := params.Validate(); err != nil {
		return "", err
	}

	salt := make([]byte, params.SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("security: read salt: %w", err)
	}

	key := deriveKey(params, []byte(password), salt)

	var b strings.Builder
	b.WriteString("$argon2id$v=19$m=")
	b.WriteString(strconv.FormatUint(uint64(params.Memory), 10))
	b.WriteString(",t=")
	b.WriteString(strconv.FormatUint(uint64(params.Iterations), 10))
	b.WriteString(",p=")
	b.WriteString(strconv.FormatUint(uint64(params.Parallelism), 10))
	b.WriteString("$")
	b.WriteString(base64.RawStdEncoding.EncodeToString(salt))
	b.WriteString("$")
	b.WriteString(base64.RawStdEncoding.EncodeToString(key))

	return b.String(), nil
}

// VerifyPassword checks a password against an encoded hash.
//
// The comparison is constant time, and the function distinguishes "wrong
// password" from "corrupt hash" so the caller never turns a database problem
// into a misleading 401.
func VerifyPassword(password, encoded string) error {
	params, salt, key, err := decodeHash(encoded)
	if err != nil {
		return err
	}

	if subtle.ConstantTimeCompare(key, deriveKey(params, []byte(password), salt)) != 1 {
		return ErrWrongPassword
	}

	return nil
}

// NeedsRehash reports whether an existing hash was produced with weaker
// settings than the current defaults, which is the signal to upgrade it on the
// next successful login.
func NeedsRehash(encoded string) bool {
	params, _, _, err := decodeHash(encoded)
	if err != nil {
		return false
	}

	defaults := DefaultParameters

	return params != defaults
}

// deriveKey is the single place argon2 is called, which keeps the swap to a
// hardware-backed KMS a one-function change. argon2.IDKey is argon2id, the only
// variant of the three that resists both side channel and GPU attacks.
func deriveKey(params Parameters, password, salt []byte) []byte {
	return argon2.IDKey(password, salt, params.Iterations, params.Memory, params.Parallelism, params.KeyLength)
}

// decodeHash parses "$argon2id$v=19$m=..,t=..,p=..$salt$hash".
func decodeHash(encoded string) (params Parameters, salt, key []byte, err error) {
	fields := strings.Split(strings.TrimSpace(encoded), "$")
	if len(fields) != 6 {
		return params, nil, nil, ErrInvalidHash
	}

	if fields[0] != "" {
		return params, nil, nil, ErrInvalidHash
	}

	if fields[1] != "argon2id" {
		return params, nil, nil, ErrUnsupportedAlgorithm
	}

	if fields[2] != "v=19" {
		return params, nil, nil, ErrUnsupportedAlgorithm
	}

	params = DefaultParameters
	for _, setting := range strings.Split(fields[3], ",") {
		name, value, found := strings.Cut(setting, "=")
		if !found {
			return params, nil, nil, ErrInvalidHash
		}

		number, parseErr := strconv.ParseUint(strings.TrimSpace(value), 10, 32)
		if parseErr != nil {
			return params, nil, nil, ErrInvalidHash
		}

		switch name {
		case "m":
			params.Memory = uint32(number)
		case "t":
			params.Iterations = uint32(number)
		case "p":
			params.Parallelism = uint8(number)
		default:
			return params, nil, nil, ErrInvalidHash
		}
	}

	if salt, err = base64.RawStdEncoding.DecodeString(fields[4]); err != nil {
		return params, nil, nil, ErrInvalidHash
	}

	if key, err = base64.RawStdEncoding.DecodeString(fields[5]); err != nil {
		return params, nil, nil, ErrInvalidHash
	}

	// The PHC string carries m, t and p only, so the key and the salt lengths
	// are taken from the decoded values. Without this a hash produced with a
	// non default profile would derive a key of the wrong size and never verify.
	params.KeyLength = uint32(len(key))
	params.SaltLength = uint32(len(salt))

	return params, salt, key, nil
}
