package auth

import (
	"golang.org/x/crypto/bcrypt"
)

func HashPassword(password string) (string, error) { 	
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)

	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func VerifyPassword(password string, encodedHash string) error {
	err := 	bcrypt.CompareHashAndPassword([]byte(encodedHash), []byte(password))

	if err != nil {
		return err
	}
	return nil
}
