package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestVerifyGitHubSignature(t *testing.T) {
	secret := "my-secret-key"
	payload := []byte(`{"ref":"refs/heads/main","repository":{"name":"test-repo"}}`)

	// Compute valid MAC
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	validSignature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	// Test valid signature
	if err := VerifyGitHubSignature(secret, validSignature, payload); err != nil {
		t.Fatalf("expected signature to be valid, got: %v", err)
	}

	// Test invalid signature
	if err := VerifyGitHubSignature(secret, "sha256=invalidhex", payload); err == nil {
		t.Fatalf("expected error for invalid hex signature")
	}

	// Test wrong secret
	if err := VerifyGitHubSignature("wrong-secret", validSignature, payload); err == nil {
		t.Fatalf("expected error for mismatched signature")
	}

	// Test missing signature
	if err := VerifyGitHubSignature(secret, "", payload); err != ErrMissingSignature {
		t.Fatalf("expected ErrMissingSignature, got: %v", err)
	}

	// Test empty secret in config
	if err := VerifyGitHubSignature("", validSignature, payload); err == nil {
		t.Fatalf("expected error when secret is empty")
	}
}
