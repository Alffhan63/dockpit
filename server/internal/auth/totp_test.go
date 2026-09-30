package auth

import (
	"testing"
	"time"
)

// RFC 6238 test vector (SHA1, secret "12345678901234567890"), truncated to 6 digits.
func TestTOTPRFCVector(t *testing.T) {
	secret := b32.EncodeToString([]byte("12345678901234567890"))
	got, err := totpAt(secret, 59/30)
	if err != nil || got != "287082" {
		t.Fatalf("totpAt = %q, %v; want 287082", got, err)
	}
	step, ok := VerifyTOTP(secret, "287082", time.Unix(59, 0))
	if !ok || step != 1 {
		t.Fatalf("VerifyTOTP = %d, %v", step, ok)
	}
	if _, ok := VerifyTOTP(secret, "287082", time.Unix(59+300, 0)); ok {
		t.Fatal("old code accepted")
	}
	if _, ok := VerifyTOTP(secret, "12345", time.Unix(59, 0)); ok {
		t.Fatal("short code accepted")
	}
}
