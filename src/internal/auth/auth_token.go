package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

func newToken() (string, error) {
	var randomBytes[32] byte

	_, err := rand.Read(randomBytes[:])
	if err != nil {
		return "", err
	}

	token := base64.RawURLEncoding.EncodeToString(randomBytes[:])
	return token, nil
}

func tokenHash(token string) string {
	tokenBytes := []byte(token)
	hash := sha256.Sum256(tokenBytes)

	hashBytes := hash[:]
	hashString := hex.EncodeToString(hashBytes)
	return hashString
}