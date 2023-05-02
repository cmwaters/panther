package consensus

import (
	"context"
	"fmt"
)

type Proposal struct {
	Data []byte
	Height uint64
	Round uint32
	Signature []byte
}

func (e *Engine) propose(
	ctx context.Context,
	height uint64,
	round, proposalRound uint32,
) error {
	// request data from the application to be proposed to the network
	proposedData, err := e.app.Read(ctx, height)
	if err != nil {
		return fmt.Errorf("requesting application for proposal data: %w", err)
	}

	return e.broadcastProposal(ctx, height, round, proposalRound, proposedData)
}

func (e *Engine) broadcastProposal(
	ctx context.Context,
	height uint64,
	round, proposalRound uint32,
	data []byte,
) error {
	if e.signer == nil {
		return nil
	}

	dataDigest := e.verifier.Hash(data)
	proposal := &Proposal{
		Height: height,
		Round:  round,
		Data:   data,
	}

	sigMsg := e.verifier.ProposalMessageBytes(proposal)

	signerResp, err := e.signer.Sign(ctx, &SignRequest{
		Type:     SignRequest_PROPOSE,
		Height:   height,
		Round:    round,
		Proposal: proposalRound,
		Message:  sigMsg,
	})
	if err != nil {
		return fmt.Errorf("failed to sign proposal: %w", err)
	}
	if signerResp.Abort {
		// The signer is indicating that it has already signed for
		// a different proposal at this height and round. Either this
		// was signed by the node but it crashed or signed by another
		// replica instance. In both cases their is a high chance that
		// it is being broadcasted through the network. If it has failed
		// to propagate then it is lost and another node will eventually
		// broadcast a new proposal.
		return nil
	}

	proposal.Signature = signerResp.Signature

	_, err = e.gossip.BroadcastProposal(ctx, &BroadcastProposalRequest{
		Proposal: proposal,
	})
	if err != nil {
		return fmt.Errorf("failed to broadcast proposal: %w", err)
	}

	// Add our own proposal immediately
	e.tally.AddProposal(proposal)

	if round == 0 {
		// If this is the first round we can also immediately vote
		return e.broadcastVote(ctx, height, round, round, dataDigest, Vote_COMMIT)
	}

	return nil
}
