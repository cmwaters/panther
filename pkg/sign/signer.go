package sign

import (
	"context"
	"fmt"
)

// Signer is a service that securely manages a nodes private key
// and signs votes and proposals for the consensus engine.
//
// The signer should ensure that the node never double signs. This usually means
// implementing a high-water mark tracking the height, round and type of vote.
//
// Make sure the verify function corresponds to the signature scheme used by
// the signer
type Signer interface {
	// ID should return a unique identifier for the signer that can be used
	// to identify the member within the group. This must always return the same
	// value
	ID() []byte

	Sign(ctx context.Context, level Watermark, msg []byte) ([]byte, error)
}

type ErrAlreadySigned []uint64

func (e ErrAlreadySigned) Error() string {
	return fmt.Sprintf("already signed msg at mark %d", e)
}

type Watermark []uint64

func (w Watermark) Greater(other Watermark) bool {
	for idx, v := range w {
		if idx >= len(other) {
			return true
		}
		if v > other[idx] {
			return true
		}
		if v < other[idx] {
			return false
		}
	}
	return false
}
