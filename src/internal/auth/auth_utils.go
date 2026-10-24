package auth

import (
	"errors"
)

var (
	ErrInvalidPasswordlength= errors.New("password must contain 1-72 bytes")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUnauthenticated = errors.New("invalid session")
	ErrCSRF = errors.New("invalid csrf token")
	errNotFound = errors.New("data record not found")
	errRotationConflict = errors.New("session rotation conflict")	
)

