package mfax

import (
	"strings"
	"testing"
	"time"
)

func TestVerifyTOTP_RFC6238_SHA1(t *testing.T) {
	// Secret = "12345678901234567890" in base32 (RFC 6238 test vectors).
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"

	if !VerifyTOTP(secret, "94287082", time.Unix(59, 0), 8, 30*time.Second, 0) {
		t.Fatalf("expected valid code for t=59")
	}
	if !VerifyTOTP(secret, "07081804", time.Unix(1111111109, 0), 8, 30*time.Second, 0) {
		t.Fatalf("expected valid code for t=1111111109")
	}
	if VerifyTOTP(secret, "00000000", time.Unix(59, 0), 8, 30*time.Second, 0) {
		t.Fatalf("expected invalid code")
	}
}

func TestGenerateSecretBase32(t *testing.T) {
	sec, err := GenerateSecretBase32(20)
	if err != nil {
		t.Fatalf("GenerateSecretBase32: %v", err)
	}
	if sec == "" {
		t.Fatalf("secret empty")
	}
	if strings.Contains(sec, "=") {
		t.Fatalf("expected no padding, got %q", sec)
	}
	if sec != strings.ToUpper(sec) {
		t.Fatalf("expected uppercase, got %q", sec)
	}
}
