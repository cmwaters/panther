package signer

import (
	"context"
	"crypto/ed25519"

	"github.com/cmwaters/halo/pkg/group"
)

type TestSigner struct {
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
	watermark  Watermark
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

func (s *TestSigner) Sign(ctx context.Context, watermark Watermark, msg []byte) ([]byte, error) {
	if !watermark.Greater(s.watermark) {
		return nil, ErrAlreadySigned(s.watermark)
	}
	s.watermark = watermark
	return ed25519.Sign(s.privateKey, msg), nil
}

func (s *TestSigner) PublicKey() ed25519.PublicKey {
	return s.publicKey
}

func (s *TestSigner) Watermark() Watermark {
	return s.watermark
}

func (s *TestSigner) ToMember(weight uint32) group.Member {
	return group.NewWeightedMember(s.publicKey, weight, group.DefaultVerifyFunc())
}
