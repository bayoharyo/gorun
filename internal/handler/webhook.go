package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrMissingSignature = errors.New("missing X-Hub-Signature-256 header")
	ErrInvalidSignature = errors.New("invalid signature")
)

// VerifyGitHubSignature validates that the given payload matches the GitHub X-Hub-Signature-256.
func VerifyGitHubSignature(secret string, signatureHeader string, payload []byte) error {
	if secret == "" {
		// If no secret is configured, reject for security
		return errors.New("webhook secret not configured for this application")
	}

	if signatureHeader == "" {
		return ErrMissingSignature
	}

	const prefix = "sha256="
	if !strings.HasPrefix(signatureHeader, prefix) {
		return fmt.Errorf("%w: missing sha256= prefix", ErrInvalidSignature)
	}

	actualMAC, err := hex.DecodeString(strings.TrimPrefix(signatureHeader, prefix))
	if err != nil {
		return fmt.Errorf("%w: unable to decode hex", ErrInvalidSignature)
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expectedMAC := mac.Sum(nil)

	if !hmac.Equal(actualMAC, expectedMAC) {
		return ErrInvalidSignature
	}

	return nil
}
