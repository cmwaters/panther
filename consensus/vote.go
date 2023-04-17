package consensus

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/protobuf/proto"
)

func (e *Engine) broadcastVote(
	ctx context.Context,
	height uint64,
	round uint32,
	proposalRound uint32,
	dataDigest []byte,
	voteType Vote_Type,
) error {
	if e.signer == nil {
		return nil
	}

	sigMsgPB := &SignatureMessage{
		Type:       SignatureMessage_Type(voteType),
		Height:     int64(height),
		Round:      int32(round),
		Namespace:  e.executor.namespace,
		DataDigest: dataDigest,
	}

	sigMsg, err := proto.Marshal(sigMsgPB)
	if err != nil {
		panic(err)
	}

	signerResponse, err := e.signer.Sign(ctx, &SignRequest{
		Type:     SignRequest_Type(voteType),
		Height:   height,
		Round:    round,
		Proposal: proposalRound,
		Message:  sigMsg,
	})
	if err != nil {
		return fmt.Errorf("failed to sign vote: %w", err)
	}

	vote := &Vote{
		Type:          voteType,
		Height:        height,
		Round:         round,
		ProposalRound: signerResponse.Proposal,
		Signature:     signerResponse.Signature,
	}

	_, err = e.gossip.BroadcastVote(ctx, &BroadcastVoteRequest{Vote: vote})
	if err != nil {
		return fmt.Errorf("failed to broadcast vote %w", err)
	}

	_, hasMajority := e.tally.AddVote(vote)
	if hasMajority {

	}

	return nil
}

func (v *Vote) ValidateForm() error {
	if v.ProposalRound > v.Round {
		return fmt.Errorf("vote in round %d is for a proposal in a future round (%d)", v.Round, v.ProposalRound)
	}

	if v.Type != Vote_SIGNAL || v.Type != Vote_COMMIT {
		return errors.New("vote must be of type signal or commit")
	}

	if len(v.Signature) == 0 {
		return errors.New("vote does not contain any signature")
	}

	return nil
}

func (v *Vote) IsCommit() bool {
	return v.Type == Vote_COMMIT
}

func (v *Vote) IsSignal() bool {
	return v.Type == Vote_SIGNAL
}
