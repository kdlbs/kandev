package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// argon2id parameters. Chosen per OWASP guidance for interactive logins:
// 64 MiB memory, 1 pass, 4 lanes. Encoded into the PHC string so parameters
// can be raised later without invalidating existing hashes.
const (
	argonTime    = 1
	argonMemory  = 64 * 1024 // KiB
	argonThreads = 4
	argonKeyLen  = 32
	argonSaltLen = 16
)

// ErrHashMismatch is returned when a password does not match its stored hash.
var ErrHashMismatch = errors.New("password does not match")

var errMalformedHash = errors.New("malformed password hash")

// HashPassword derives an argon2id hash and encodes it as a PHC string:
// $argon2id$v=19$m=65536,t=1,p=4$<b64 salt>$<b64 key>
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// VerifyPassword checks password against a PHC-encoded argon2id hash.
// Returns ErrHashMismatch when the password is wrong.
func VerifyPassword(password, encoded string) error {
	memory, timeCost, threads, salt, key, err := decodePHC(encoded)
	if err != nil {
		return err
	}
	candidate := argon2.IDKey([]byte(password), salt, timeCost, memory, threads, uint32(len(key)))
	if subtle.ConstantTimeCompare(candidate, key) != 1 {
		return ErrHashMismatch
	}
	return nil
}

func decodePHC(encoded string) (memory, timeCost uint32, threads uint8, salt, key []byte, err error) {
	parts := strings.Split(encoded, "$")
	// ["", "argon2id", "v=19", "m=...,t=...,p=...", salt, key]
	if len(parts) != 6 || parts[1] != "argon2id" {
		return 0, 0, 0, nil, nil, errMalformedHash
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return 0, 0, 0, nil, nil, errMalformedHash
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &timeCost, &threads); err != nil {
		return 0, 0, 0, nil, nil, errMalformedHash
	}
	salt, err = base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return 0, 0, 0, nil, nil, errMalformedHash
	}
	key, err = base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(key) == 0 {
		return 0, 0, 0, nil, nil, errMalformedHash
	}
	return memory, timeCost, threads, salt, key, nil
}
