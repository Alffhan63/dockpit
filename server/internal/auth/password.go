package auth

import (
	"errors"

	"golang.org/x/crypto/bcrypt"
)

// MinPasswordLength is the minimum admin password length.
const MinPasswordLength = 12

// HashPassword returns a bcrypt hash of password.
func HashPassword(password string) (string, error) {
	if len(password) < MinPasswordLength {
		return "", errors.New("password must be at least 12 characters")
	}
	if len(password) > 72 {
		return "", errors.New("password must be at most 72 bytes") // bcrypt limit
	}
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(h), err
}

// CheckPassword reports whether password matches hash.
func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
