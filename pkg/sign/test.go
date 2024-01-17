package sign

import (
	"context"
	"crypto/ed25519"

	"github.com/cmwaters/panther/pkg/group"
)

type TestSigner struct {
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
	level      Watermark
}

func NewTestSigner() *TestSigner {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		panic(err)
	}
	return &TestSigner{
		privateKey: priv,
		publicKey:  pub,
	}
}

func (s *TestSigner) Sign(ctx context.Context, level Watermark, msg []byte) ([]byte, error) {
	if !level.Greater(s.level) {
		return nil, ErrAlreadySigned(s.level)
	}
	s.level = level
	return ed25519.Sign(s.privateKey, msg), nil
}

func (s *TestSigner) ID() []byte {
	return s.publicKey
}

func (s *TestSigner) PublicKey() ed25519.PublicKey {
	return s.publicKey
}

func (s *TestSigner) Level() Watermark {
	return s.level
}

func (s *TestSigner) ToMember(weight uint32) group.Member {
	return group.NewWeightedMember(s.publicKey, weight, group.DefaultVerifyFunc())
}
