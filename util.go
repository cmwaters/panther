package consensus

import (
	"crypto/ed25519"
	"fmt"

	"google.golang.org/protobuf/proto"
)

// Default to ed25519
func DefaultVerifyFunc() VerifyFunc {
	return func(publicKey, message, signature []byte) bool {
		return ed25519.Verify(publicKey, message, signature)
	}
}

func CannonicalizedVoteBytes(v *Vote, payloadID []byte, namespace string) []byte {
	if v == nil {
		panic("nil vote")
	}
	msg := &SignRequest{
		Type:      SignRequest_Type(v.Type),
		Height:    v.Height,
		Round:     v.Round,
		PayloadId: payloadID,
		Namespace: namespace,
	}
	bz, err := proto.Marshal(msg)
	if err != nil {
		panic(fmt.Errorf("marshal sign request bytes: %w", err))
	}
	return bz
}
