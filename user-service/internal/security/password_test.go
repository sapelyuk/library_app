package security_test

import (
	"errors"
	"strings"
	"testing"

	"library_app/user-service/internal/security"
)

// fastParameters keeps the tests quick: they exercise the encoding and the
// verification logic, not the cost profile, and the profile itself is checked
// by TestParametersValidate.
var fastParameters = security.Parameters{
	Memory:      8 * 1024,
	Iterations:  1,
	Parallelism: 1,
	KeyLength:   16,
	SaltLength:  8,
}

func TestParametersValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		params  security.Parameters
		wantErr bool
	}{
		{name: "defaults are valid", params: security.DefaultParameters},
		{name: "fast test profile is valid", params: fastParameters},
		{name: "memory below the floor", params: withMemory(fastParameters, 8*1024-1), wantErr: true},
		{name: "zero iterations", params: withIterations(fastParameters, 0), wantErr: true},
		{name: "zero parallelism", params: withParallelism(fastParameters, 0), wantErr: true},
		{name: "key shorter than the floor", params: withKeyLength(fastParameters, 15), wantErr: true},
		{name: "salt shorter than the floor", params: withSaltLength(fastParameters, 7), wantErr: true},
	}

	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.params.Validate()

			if tc.wantErr && err == nil {
				t.Fatal("want an error, got nil")
			}

			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestParametersString(t *testing.T) {
	t.Parallel()

	got := security.DefaultParameters.String()
	want := "argon2id$v=19$m=19456,t=2,p=1,k=32,s=16"

	if got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}

func TestHashPasswordRoundTrip(t *testing.T) {
	t.Parallel()

	const password = "Str0ng-Pass!"

	encoded, err := security.HashPasswordWith(fastParameters, password)
	if err != nil {
		t.Fatalf("HashPasswordWith: unexpected error: %v", err)
	}

	if err := security.VerifyPassword(password, encoded); err != nil {
		t.Fatalf("the correct password must verify, got %v", err)
	}

	if err := security.VerifyPassword(password+"x", encoded); !errors.Is(err, security.ErrWrongPassword) {
		t.Fatalf("want ErrWrongPassword, got %v", err)
	}
}

func TestHashPasswordIsSalted(t *testing.T) {
	t.Parallel()

	const password = "Str0ng-Pass!"

	first, err := security.HashPasswordWith(fastParameters, password)
	if err != nil {
		t.Fatalf("HashPasswordWith: unexpected error: %v", err)
	}

	second, err := security.HashPasswordWith(fastParameters, password)
	if err != nil {
		t.Fatalf("HashPasswordWith: unexpected error: %v", err)
	}

	if first == second {
		t.Fatal("two hashes of the same password must differ thanks to the random salt")
	}

	if err := security.VerifyPassword(password, second); err != nil {
		t.Fatalf("the second hash must verify too, got %v", err)
	}
}

func TestHashPasswordEncoding(t *testing.T) {
	t.Parallel()

	encoded, err := security.HashPasswordWith(fastParameters, "Str0ng-Pass!")
	if err != nil {
		t.Fatalf("HashPasswordWith: unexpected error: %v", err)
	}

	if !strings.HasPrefix(encoded, "$argon2id$v=19$m=8192,t=1,p=1$") {
		t.Fatalf("unexpected prefix: %q", encoded)
	}

	if fields := strings.Split(encoded, "$"); len(fields) != 6 {
		t.Fatalf("a PHC hash must have 6 $-separated fields, got %d in %q", len(fields), encoded)
	}
}

func TestHashPasswordWithRejectsBadParameters(t *testing.T) {
	t.Parallel()

	if _, err := security.HashPasswordWith(withIterations(fastParameters, 0), "Str0ng-Pass!"); err == nil {
		t.Fatal("want an error for invalid parameters, got nil")
	}
}

func TestHashPasswordUsesDefaults(t *testing.T) {
	t.Parallel()

	const password = "Str0ng-Pass!"

	encoded, err := security.HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword: unexpected error: %v", err)
	}

	if err := security.VerifyPassword(password, encoded); err != nil {
		t.Fatalf("the default hash must verify, got %v", err)
	}

	if security.NeedsRehash(encoded) {
		t.Fatal("a hash produced with the current defaults must not need a rehash")
	}
}

func TestNeedsRehash(t *testing.T) {
	t.Parallel()

	weak, err := security.HashPasswordWith(fastParameters, "Str0ng-Pass!")
	if err != nil {
		t.Fatalf("HashPasswordWith: unexpected error: %v", err)
	}

	if !security.NeedsRehash(weak) {
		t.Fatal("a hash produced with weaker parameters must need a rehash")
	}

	if security.NeedsRehash("not a hash") {
		t.Fatal("a malformed hash must not be reported as needing a rehash")
	}
}

func TestVerifyPasswordMalformedHashes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		encoded string
		wantErr error
	}{
		{name: "empty", encoded: "", wantErr: security.ErrInvalidHash},
		{name: "garbage", encoded: "not-a-hash", wantErr: security.ErrInvalidHash},
		{name: "missing the hash part", encoded: "$argon2id$v=19$m=8192,t=1,p=1$c2FsdA", wantErr: security.ErrInvalidHash},
		{name: "empty algorithm", encoded: "$$v=19$m=8192,t=1,p=1$c2FsdA$a2V5", wantErr: security.ErrUnsupportedAlgorithm},
		{name: "unsupported algorithm", encoded: "$argon2i$v=19$m=8192,t=1,p=1$c2FsdA$a2V5", wantErr: security.ErrUnsupportedAlgorithm},
		{name: "unsupported version", encoded: "$argon2id$v=18$m=8192,t=1,p=1$c2FsdA$a2V5", wantErr: security.ErrUnsupportedAlgorithm},
		{name: "unknown parameter", encoded: "$argon2id$v=19$m=8192,x=1,p=1$c2FsdA$a2V5", wantErr: security.ErrInvalidHash},
		{name: "parameter without a value", encoded: "$argon2id$v=19$m=8192,t,p=1$c2FsdA$a2V5", wantErr: security.ErrInvalidHash},
		{name: "non numeric parameter", encoded: "$argon2id$v=19$m=abc,t=1,p=1$c2FsdA$a2V5", wantErr: security.ErrInvalidHash},
		{name: "salt is not base64", encoded: "$argon2id$v=19$m=8192,t=1,p=1$!!!!$a2V5", wantErr: security.ErrInvalidHash},
		{name: "key is not base64", encoded: "$argon2id$v=19$m=8192,t=1,p=1$c2FsdA$!!!!", wantErr: security.ErrInvalidHash},
	}

	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := security.VerifyPassword("Str0ng-Pass!", tc.encoded)

			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("want %v, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestDummyPasswordHash(t *testing.T) {
	t.Parallel()

	// The dummy hash must be a well formed argon2id encoding, so that a login
	// attempt against a missing account spends the same CPU as a real one.
	if err := security.VerifyPassword("anything", security.DummyPasswordHash); !errors.Is(err, security.ErrWrongPassword) {
		t.Fatalf("want ErrWrongPassword, got %v", err)
	}

	security.VerifyDummy("anything")
}

func withMemory(params security.Parameters, memory uint32) security.Parameters {
	params.Memory = memory

	return params
}

func withIterations(params security.Parameters, iterations uint32) security.Parameters {
	params.Iterations = iterations

	return params
}

func withParallelism(params security.Parameters, parallelism uint8) security.Parameters {
	params.Parallelism = parallelism

	return params
}

func withKeyLength(params security.Parameters, keyLength uint32) security.Parameters {
	params.KeyLength = keyLength

	return params
}

func withSaltLength(params security.Parameters, saltLength uint32) security.Parameters {
	params.SaltLength = saltLength

	return params
}
