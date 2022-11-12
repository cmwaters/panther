package consensus

import (
	"crypto/ed25519"
	sync "sync"
)

type state struct {
	sync.RWMutex
	height uint64
	round uint32
	step uint8	

	pubkey ed25519.PublicKey
}

func newState() *state {
	return &state{}
}

func (s *state) setPubkey(pubkey ed25519.PublicKey) {
	s.pubkey = pubkey
}
