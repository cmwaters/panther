package consensus

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
)

type VoteType uint8

const (
	// There are three initial vote types necessary for the protocol:
	// Proposal, Lock and Commit.
	PROPOSAL_TYPE VoteType = iota + 1
	LOCK_TYPE
	COMMIT_TYPE

	// MaxNamespaceSize indicates the maximum length in bytes of the namespace.
	// A namespace can be empty thus 0 is accepted.
	MaxNamespaceSize = math.MaxUint8
	// MaxDataDigestSize indicates the maximum length in bytes of the data digest.
	// A data digest can not be empty
	MaxDataDigestSize = math.MaxUint8
)

var (
	ErrInvalidSignedMsgLength = errors.New("invalid signed message length")
)

// EncodeMsgToSign encodes the information to be signed over
//
// The format is:
// 1 byte vote type (also used for versioning)
// 8 bytes height
// 4 bytes round
// up to 255 bytes length prefixed data digest (single byte length)
// up to 255 bytes length prefixed namespace (single byte length)
//
// Namespace can be left empty
func EncodeMsgToSign(
	voteType uint8,
	height uint64,
	round uint32,
	dataDigest []byte,
	namespace []byte,
) []byte {
	buf := bytes.NewBuffer(nil)
	buf.WriteByte(voteType)
	heightBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(heightBytes, height)
	buf.Write(heightBytes)
	roundBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(roundBytes, round)
	buf.Write(roundBytes)

	if dataDigest == nil || len(dataDigest) == 0 {
		panic("data digest can not be empty")
	}
	if len(dataDigest) > MaxDataDigestSize {
		panic("data digest can not be longer than 255 bytes")
	}
	buf.WriteByte(byte(len(dataDigest)))
	buf.Write(dataDigest)
	if len(namespace) > MaxNamespaceSize {
		panic("namespace can not be longer than 255 bytes")
	}
	buf.WriteByte(byte(len(namespace)))
	if len(namespace) > 0 {
		buf.Write(namespace)
	}
	return buf.Bytes()
}

// DecodeSignedMsg reverses the encoding scheme, taking the raw
// bytes and returning the vote type, height, round, data digest,
// and namespace.
func DecodeSignedMsg(msg []byte) (
	voteType uint8,
	height uint64,
	round uint32,
	dataDigest []byte,
	namespace []byte,
	err error,
) {
	if len(msg) < 14 {
		return 0, 0, 0, nil, nil, ErrInvalidSignedMsgLength
	}

	voteType = msg[0]
	height = binary.BigEndian.Uint64(msg[1:9])
	round = binary.BigEndian.Uint32(msg[9:13])
	dataDigestLength := msg[13]
	if len(msg) < 14+int(dataDigestLength)+1 {
		return 0, 0, 0, nil, nil, ErrInvalidSignedMsgLength
	}
	dataDigest = msg[14 : 14+dataDigestLength]
	namespaceLength := dataDigest[14+dataDigestLength]
	if len(msg) < 14+int(dataDigestLength)+1+int(namespaceLength) {
		return 0, 0, 0, nil, nil, ErrInvalidSignedMsgLength
	}
	namespace = msg[14+dataDigestLength+1 : 14+dataDigestLength+1+namespaceLength]
	return
}
