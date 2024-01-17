package group

import "fmt"

// Commitment is an interface encapsulating a groups commitment to a value. It is the product of
// an instance of consensus and can be used by other processes not involved in consensus itself
// to verify values under that security model.
type Commitment interface {
	// Verify takes a group and verifies that sufficient members of the group signed the commitment
	Verify(group Group, value []byte) error

	// Signers returns the list of member indexes that signed the commitment
	Signers() []uint32
}

type SignatureSet struct {
	signatures [][]byte
}

func NewSignatureSet(signatures [][]byte) Commitment {
	return &SignatureSet{
		signatures: signatures,
	}
}

func (s *SignatureSet) Verify(group Group, value []byte) error {
	if group.Size() != len(s.signatures) {
		return fmt.Errorf("invalid signature size, expected %d, got %d", group.Size(), len(s.signatures))
	}
	totalWeight := group.TotalWeight()
	currentWeight := uint64(0)
	for i, sig := range s.signatures {
		if sig == nil {
			continue
		}
		member := group.Member(uint(i))
		if !member.Verify(value, sig) {
			return fmt.Errorf("failed to verify signature %X for member %X", sig, member.ID())
		}
		currentWeight += uint64(member.Weight())
		if currentWeight*2 > totalWeight*3 {
			return nil
		}
	}
	return fmt.Errorf("not enough votes to verify commitment to value, required: %d, got: %d", totalWeight*2/3, currentWeight)
}

func (s *SignatureSet) Signers() []uint32 {
	signers := make([]uint32, 0, len(s.signatures))
	for i, sig := range s.signatures {
		if sig != nil {
			signers = append(signers, uint32(i))
		}
	}
	return signers
}
