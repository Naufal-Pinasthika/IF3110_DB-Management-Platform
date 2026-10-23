package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

func newToken() (string, error) {
	// genertae 32 byte (256 bit)	
	var randomBytes[32] byte

	// generate random from rand.Read()
	// uses _ cause we dont need n int return data from rand.Read() 
	// (we only need the randomBytes anyway because how go handle slice like pass by reference)
	_, err := rand.Read(randomBytes[:])
	if err != nil {
		return "", err
	}

	// convert to Base64 (URL save)
	token := base64.RawURLEncoding.EncodeToString(randomBytes[:])
	return token, nil
}

// hash token
func tokenHash(token string) string {
	// generate checksum hash
	tokenBytes := []byte(token)
	hash := sha256.Sum256(tokenBytes)

	// convert to string
	hashBytes := hash[:]
	hashString := hex.EncodeToString(hashBytes)
	return hashString
}