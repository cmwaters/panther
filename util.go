package consensus

import "crypto/ed25519"

// Default to ed25519
func DefaultVerifyFunc() VerifyFunc {
	return func(publicKey, message, signature []byte) bool {
		return ed25519.Verify(publicKey, message, signature)
	}
}
