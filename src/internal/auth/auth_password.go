package auth

import (
	"golang.org/x/crypto/bcrypt"
)

// main reference: https://gowebexamples.com/password-hashing/

// Hashes platform account password
func HashPassword(password string) (string, error) { 	
	// validation 0 < len(password) <= 72 (pick 72 because bcrypt has a strict limit 72 bytes)
	if len(password) <= 0 || len(password) > 72 {
		return "", ErrInvalidPasswordlength
	}
	
	// uses 12 optimal work factor
	// https://stackoverflow.com/questions/4443476/optimal-bcrypt-work-factor
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)

	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// Checks password relative to its hash
func VerifyPassword(password string, encodedHash string) error {
	err := 	bcrypt.CompareHashAndPassword([]byte(encodedHash), []byte(password))

	if err != nil {
		return err
	}
	return nil
}
