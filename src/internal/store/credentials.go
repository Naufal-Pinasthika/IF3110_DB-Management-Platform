package store

import "github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/apperror"

type Cipher struct {
	Key []byte
}

// Encrypt protects target passwords that the backend must later recover.
func (c *Cipher) Encrypt(plaintext string) (string, error) {
	// TODO Use authenticated encryption such as AES GCM with a fresh random nonce.
	// TODO Store the nonce with ciphertext using a documented encoding.
	// TODO Keep the key in the environment and never log any secret.
	return "", apperror.ErrNotImplemented
}

// Decrypt recovers credentials only when opening a target connection.
func (c *Cipher) Decrypt(ciphertext string) (string, error) {
	// TODO Decode the payload and verify its length and authentication tag.
	// TODO Reject invalid data without including ciphertext or plaintext in errors.
	return "", apperror.ErrNotImplemented
}
