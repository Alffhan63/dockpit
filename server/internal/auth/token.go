// Package auth holds authentication helpers for the controller.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"
)

// BearerToken extracts the token from an "Authorization: Bearer <token>" header.
func BearerToken(r *http.Request) (string, bool) {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	token = strings.TrimSpace(token)
	return token, ok && token != ""
}

// TokenEqual compares two tokens in constant time. Both sides are hashed first
// so the comparison does not leak the token length.
func TokenEqual(a, b string) bool {
	ha := sha256.Sum256([]byte(a))
	hb := sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(ha[:], hb[:]) == 1
}

// NewToken returns a random 256-bit token, hex encoded.
func NewToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand never fails on supported platforms
	}
	return hex.EncodeToString(b)
}

// HashToken returns the SHA-256 of a high-entropy token for storage.
// A fast hash is fine here: tokens are random, not user-chosen.
func HashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
