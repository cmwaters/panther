package consensus

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/cmwaters/panther/pkg/app"
	"github.com/cmwaters/panther/pkg/group"
	"github.com/cmwaters/panther/pkg/sign"
)

// proposeFn generates a function that the tally will call whenever it reaches a stage in the
// protocol where it needs to propose. Concretely, this will be called whenever the Tally enters
// a new round. Only if the node is the proposer for that round will it actually formulate a
// proposal and gossip it to the network.
func (e *Engine) proposeFn(
	height uint64,
	group group.Group,
	store *Store,
	propose app.Propose,
) ProposeFn {
	if e.signer == nil {
		// cannot propose without a signer
		return nil
	}
	id := e.signer.ID()

	return func(ctx context.Context, round uint32) (bool, error) {
		if !bytes.Equal(group.Proposer(uint(round)).ID(), id) {
			// this process is not the proposer for this round
			return false, nil
		}

		// call the application to compose the data within the proposal
		data, err := propose(ctx, height)
		if err != nil {
			return false, fmt.Errorf("requesting application for proposal data: %w", err)
		}
		dataDigest := e.hasher.New().Sum(data)

		proposal := NewProposal(height, round, data)
		signBytes := proposal.SignBytes(dataDigest, e.namespace)
		signature, err := e.signer.Sign(ctx, proposal.Level(), signBytes)
		if err != nil {
			return false, fmt.Errorf("signing proposal: %w", err)
		}
		proposal.Signature = signature

		// sanity check that this function constructs a correctly formed proposal
		if err := proposal.ValidateForm(); err != nil {
			panic(err)
		}

		if err := e.gossip.BroadcastProposal(ctx, proposal); err != nil {
			return false, fmt.Errorf("broadcasting proposal: %w", err)
		}

		// add the proposal to the store
		store.AddProposal(proposal, dataDigest)

		return true, nil
	}
}

type Proposal struct {
	Data      []byte
	Height    uint64
	Round     uint32
	Signature []byte
}

func NewProposal(height uint64, round uint32, data []byte) *Proposal {
	return &Proposal{
		Data:   data,
		Height: height,
		Round:  round,
	}
}

func (p Proposal) SignBytes(dataDigest, namespace []byte) []byte {
	return EncodeMsgToSign(uint8(PROPOSAL_TYPE), p.Height, p.Round, dataDigest, namespace)
}

func (p Proposal) Level() sign.Watermark {
	return sign.Watermark{p.Height, uint64(p.Round), uint64(0)}
}

func (p Proposal) ValidateForm() error {
	if len(p.Signature) == 0 {
		return errors.New("proposal does not contain any signature")
	}

	if p.Height == 0 {
		return errors.New("proposal height is zero")
	}

	if p.Round == 0 {
		return errors.New("proposal round is zero")
	}

	if len(p.Data) == 0 {
		return errors.New("proposal does not contain any data")
	}

	return nil
}

func (p *Proposal) String() string {
	if p == nil {
		return "nil"
	}

	return fmt.Sprintf("Proposal{%d/%d for %X $ %X}", p.Height, p.Round, truncate(p.Data, 32), p.Signature)
}

func truncate(data []byte, max int) []byte {
	if len(data) > max {
		return data[:max]
	}
	return data
}
