package auth

import (
	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/apperror"
)

// HashPassword hashes a platform account password.
func HashPassword(password string) (string, error) {
	// TODO Choose bcrypt or Argon2id with a random salt and documented cost.
	// TODO Never use reversible encryption or a fast hash for account passwords.
	return "", apperror.ErrNotImplemented
}

// VerifyPassword checks a password with the selected hashing library.
func VerifyPassword(password, encodedHash string) error {
	// TODO Use the verifier supplied by the same library as HashPassword.
	// TODO Return a generic authentication error and never log either input.
	return apperror.ErrNotImplemented
}
