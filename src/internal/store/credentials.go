package store

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/apperror"
)

type Cipher struct {
	Key []byte
}

// returned when ciphertext is malformed or authentication fails.
var ErrInvalidCiphertext = errors.New("invalid ciphertext")


func (c *Cipher) Encrypt(plaintext string) (string, error) {
	if len(c.Key) != 32 {
		return "", apperror.New("encryption key must be 32 bytes")
	}
	if plaintext == "" {
		return "", apperror.New("plaintext cannot be empty")
	}

	block, err := aes.NewCipher(c.Key)
	if err != nil {
		return "", apperror.Wrap(err, "failed to create cipher")
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", apperror.Wrap(err, "failed to create GCM")
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", apperror.Wrap(err, "failed to generate nonce")
	}

	ciphertext := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	
	result := make([]byte, len(nonce)+len(ciphertext))
	copy(result[:len(nonce)], nonce)
	copy(result[len(nonce):], ciphertext)

	return base64.StdEncoding.EncodeToString(result), nil
}

func (c *Cipher) Decrypt(ciphertext string) (string, error) {
	if len(c.Key) != 32 {
		return "", apperror.New("encryption key must be 32 bytes")
	}
	if ciphertext == "" {
		return "", apperror.New("ciphertext cannot be empty")
	}

	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", apperror.New("ciphertext must be valid base64")
	}

	block, err := aes.NewCipher(c.Key)
	if err != nil {
		return "", apperror.Wrap(err, "failed to create cipher")
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", apperror.Wrap(err, "failed to create GCM")
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", ErrInvalidCiphertext
	}

	nonce := data[:nonceSize]
	ciphertextBytes := data[nonceSize:]

	plaintext, err := gcm.Open(nil, nonce, ciphertextBytes, nil)
	if err != nil {
		return "", ErrInvalidCiphertext
	}

	return string(plaintext), nil
}
