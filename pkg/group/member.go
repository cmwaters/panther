package group

import (
	"crypto/ed25519"
)

// Dictates how signatures from voters should be verified. This needs
// to match with the key protocol of the signer.
type VerifyFunc func(publicKey, message, signature []byte) bool

// Default to ed25519
func DefaultVerifyFunc() VerifyFunc {
	return func(publicKey, message, signature []byte) bool {
		return ed25519.Verify(publicKey, message, signature)
	}
}

var _ Member = (*WeightedMember)(nil)

type WeightedMember struct {
	pubKey []byte
	weight uint32
	verify VerifyFunc
}

func NewWeightedMember(publicKey []byte, weight uint32, verify VerifyFunc) *WeightedMember {
	return &WeightedMember{
		pubKey: publicKey,
		weight: weight,
		verify: verify,
	}
}

func (m *WeightedMember) ID() []byte {
	return m.pubKey
}

func (m *WeightedMember) Weight() uint32 {
	return m.weight
}

func (m *WeightedMember) Verify(msg, sig []byte) bool {
	return m.verify(m.pubKey, msg, sig)
}
