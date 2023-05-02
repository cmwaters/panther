package consensus

import (
	"context"
	"errors"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (e *Engine) receiveProposal(ctx context.Context) error {
	attempts := 1
	for {
		state := e.executor.state()
		proposal, err := e.gossip.ReceiveProposal(ctx)
		switch status.Code(err) {
		case codes.OK:
		// retry with exponential backoff when receiving these error codes
		case codes.Unavailable, codes.DeadlineExceeded:
			e.logger.Info().
				Uint64("height", state.Height).
				Str("response", err.Error()).
				Msg("retrying with exponential backoff")
			time.Sleep(exponentialBackoff(attempts))
			attempts++
			continue
		default:
			return fmt.Errorf("receiving proposal: %w", err)
		}

		if err := e.handleProposal(ctx, proposal, state); err != nil {
			e.logger.Err(err).
				Uint64("height", state.Height).
				Uint32("round", state.Round).
				Msg("handling proposal")
			if isUnrecoverable(err) {
				return err
			}
		}
		return nil
	}
}

func (e *Engine) receiveVotes(ctx context.Context) error {
	attempts := 1
	for {
		state := e.executor.state()
		vote, err := e.gossip.ReceiveVote(ctx)
		switch status.Code(err) {
		case codes.OK:
		// retry with exponential backoff when receiving these error codes
		case codes.Unavailable, codes.DeadlineExceeded:
			e.logger.Info().
				Uint64("height", state.Height).
				Str("response", err.Error()).
				Msg("retrying with exponential backoff")
			time.Sleep(exponentialBackoff(attempts))
			attempts++
			continue
		default:
			return fmt.Errorf("receiving proposal: %w", err)
		}

		go func() {
			if err := e.handleVote(ctx, vote, state); err != nil {
				e.logger.Err(err).
					Uint64("height", vote.Height).
					Uint32("round", vote.Round).
					Uint32("member", vote.MemberIndex).
					Msg("handling vote")
				if isUnrecoverable(err) {
					_ = e.Stop()
				}
			}
		}()
		attempts = 1
	}
}

func (e *Engine) handleProposal(ctx context.Context, proposal *Proposal, state *State) error {
	if proposal == nil {
		return errors.New("received nil proposal")
	}

	// Check that we don't already have the proposal
	if e.tally.HasProposal(proposal.Round) {
		e.logger.Info().
			Uint64("height", proposal.Height).
			Uint32("round", proposal.Round).
			Msg("proposal already in tally")
		return nil
	}

	// Verify the proposal. Including that it came from the correct proposer and
	// that the signature for the data is valid.
	if err := e.verifier.VerifyProposal(proposal, state.Height, state.Round); err != nil {
		return err
	}

	// Allow the app to verify the proposal
	err := e.app.Verify(ctx, proposal.Height, proposal.Data)
	if err != nil {
		// TODO: the proposal should be cached so that it is automatically rejected
		// if it is to be received again.
		return nil
	}

	// Add the proposal to the tally
	e.tally.AddProposal(proposal)

	// If the engine has already decided for this round then there is nothing more
	// that needs to be done. (We are waiting for votes from the rest of the network)
	if e.executor.hasDecided() {
		return nil
	}

	// If this is the first valid proposal the node has received it can immediately
	// vote for it else we wait before making a decision.
	if !e.tally.HasOnlyOneProposal() {
		return nil
	}

	dataDigest := e.verifier.Hash(proposal.Data)

	voteType := Vote_SIGNAL
	if proposal.Round == 0 {
		// The first round in every height serves as an exception whereby a member
		// can immediately vote to commit.
		voteType = Vote_COMMIT
	}

	err = e.executor.tryDecide(ctx, state.Height, state.Round, func(ctx context.Context, height uint64, round uint32) error {
		return e.broadcastVote(ctx, height, round, proposal.Round, dataDigest, voteType)
	})
	if err != nil {
		return unrecoverable(err)
	}
	return nil
}

func (e *Engine) handleVote(ctx context.Context, vote *Vote, state *State) error {
	if vote == nil {
		return errors.New("received nil vote")
	}

	// Check that we don't already have the vote
	if e.tally.HasVote(vote) {
		e.logger.Info().
			Uint64("height", vote.Height).
			Uint32("round", vote.Round).
			Msg("vote is already in tally")
		return nil
	}

	// Get the proposal to verify the vote against
	proposalID, proposal := e.tally.GetProposal(vote.ProposalRound)
	if proposalID == nil {
		if vote.Height == state.Height {
			e.tally.AddPendingVote(vote)
		}
		// TODO: We may want to consider handling votes at the height directly above
		// the nodes current state.
		return nil
	}

	// verify the vote's signature and other components
	if err := e.verifier.VerifyVote(vote, state.Height, proposalID); err != nil {
		return err
	}

	hasQuorum, added := e.tally.AddVote(vote)
	if !added || !hasQuorum {
		// there will have been no state transition so we can exit early
		return nil
	}

	// The node has received a majority of votes from others. A decision may
	// therefore be reached.
	if err := e.handleQuorum(ctx, vote.Round, state, proposal); err != nil {
		return unrecoverable(err)
	}
	return nil
}

func exponentialBackoff(attempts int) time.Duration {
	// 40, 160, 360, 640, 1000, 1440, 1960
	return time.Duration(attempts*attempts*40) * time.Millisecond
}
