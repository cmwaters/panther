package consensus

import (
	"context"
	"crypto"
	"errors"
	"fmt"

	"github.com/cmwaters/panther/pkg/app"
	"github.com/cmwaters/panther/pkg/group"
)

type Verifier struct {
	height           uint64
	namespace        []byte
	group            group.Group
	hasher           crypto.Hash
	verifyProposalFn app.VerifyProposal
}

func NewVerifier(namespace []byte, height uint64, group group.Group, hasher crypto.Hash, verifyProposalFn app.VerifyProposal) *Verifier {
	return &Verifier{
		height:           height,
		namespace:        namespace,
		group:            group,
		hasher:           hasher,
		verifyProposalFn: verifyProposalFn,
	}
}

func (v *Verifier) Update(group group.Group, height uint64) {
	v.group = group
	v.height = height
}

func (v *Verifier) GetProposer(round uint32) group.Member {
	return v.group.Proposer(uint(round))
}

func (v *Verifier) GetMember(index uint32) group.Member {
	return v.group.Member(uint(index))
}

func (v *Verifier) VerifyProposal(ctx context.Context, proposal *Proposal) ([]byte, error) {
	if len(proposal.Signature) == 0 {
		return nil, errors.New("proposal signature missing")
	}

	if v.height != proposal.Height {
		return nil, fmt.Errorf("proposal is from a different height (exp: %d, got: %d)", v.height, proposal.Height)
	}

	errCh := make(chan error, 1)
	// start a separate goroutine to verify the proposal. This can run in parallel with the
	// signature verification
	go func() {
		errCh <- v.verifyProposalFn(ctx, proposal.Height, proposal.Data)
	}()

	proposer := v.group.Proposer(uint(proposal.Round))
	dataDigest := v.Hash(proposal.Data)
	if proposer.Verify(proposal.SignBytes(dataDigest, v.namespace), proposal.Signature) {
		// This could be caused by one of a few things:
		// - The proposal comes from a member who is not currently the proposer
		// - The proposer has incorrectly signed a different set of data
		// - There has been a breaking change to how a proposal message is serialized
		return nil, errors.New("invalid proposal signature")
	}

	// block until the application has verified the proposal
	if err := <-errCh; err != nil {
		return nil, err
	}

	return dataDigest, nil
}

func (v *Verifier) Hash(data []byte) []byte {
	hash := v.hasher.New()
	return hash.Sum(data)
}

func (v *Verifier) VerifyVote(vote *Vote, dataDigest []byte) error {
	if err := vote.ValidateForm(); err != nil {
		return err
	}

	if vote.Height != v.height {
		return fmt.Errorf("vote is from a different height (exp: %d, got: %d)", v.height, vote.Height)
	}

	if int(vote.MemberIndex) >= v.group.Size() {
		return fmt.Errorf("invalid member index exceeds total members (%d)", vote.MemberIndex)
	}

	member := v.group.Member(uint(vote.MemberIndex))
	if !member.Verify(vote.SignBytes(dataDigest, v.namespace), vote.Signature) {
		return errors.New("invalid vote signature")
	}

	return nil
}

func (v *Verifier) Height() uint64 {
	return v.height
}

func (v *Verifier) Namespace() []byte {
	return v.namespace
}
