package consensus

import (
	"context"
	"errors"
	"fmt"
)

func (e *Engine) receiveProposals(
	ctx context.Context,
	executor *Executor,
	verifier *Verifier,
	store *Store,
) error {
	height := verifier.Height()
	for {
		proposal, err := e.gossip.ReceiveProposal(ctx, height)
		if err != nil {
			e.logger.Err(err).
				Uint64("height", height).
				Msg("receiving proposal")
			_ = e.Stop()
		}

		if proposal == nil {
			return errors.New("received nil proposal")
		}

		if proposal.Height != height {
			return fmt.Errorf("received proposal for wrong height, exp %d, got %d", height, proposal.Height)
		}

		err = e.handleProposal(ctx, proposal, executor, verifier, store)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			e.logger.Info().
				Err(err).
				Uint64("height", height).
				Msg("invalid proposal")

			err = e.gossip.ReportProposal(ctx, proposal)
			if err != nil {
				e.logger.Err(err).Msg("reporting proposal")
			}
		}
	}
}

func (e *Engine) receiveVotes(
	ctx context.Context,
	executor *Executor,
	verifier *Verifier,
	store *Store,
) error {
	height := verifier.Height()
	for {
		vote, err := e.gossip.ReceiveVote(ctx, height)
		if err != nil {
			return err
		}

		if vote == nil {
			return errors.New("received nil vote")
		}

		if vote.Height != height {
			return fmt.Errorf("received vote for wrong height, exp %d, got %d", height, vote.Height)
		}

		go func() {
			err := e.handleVote(ctx, vote, executor, verifier, store)
			if err != nil {
				// check if the context was cancelled
				if ctx.Err() != nil {
					return
				}
				e.logger.Info().
					Err(err).
					Uint64("height", height).
					Str("vote", vote.String()).
					Msg("invalid vote")

				err = e.gossip.ReportVote(ctx, vote)
				if err != nil {
					e.logger.Err(err).Msg("reporting vote")
				}

			}
		}()
	}
}

func (e *Engine) handleProposal(
	ctx context.Context,
	proposal *Proposal,
	executor *Executor,
	verifier *Verifier,
	store *Store,
) error {
	if err := proposal.ValidateForm(); err != nil {
		return err
	}

	// Check that we don't already have the proposal
	if store.HasProposal(proposal.Round) {
		e.logger.Info().
			Uint64("height", proposal.Height).
			Uint32("round", proposal.Round).
			Msg("proposal already in executor")
		return nil
	}

	// Verify the proposal. Including that it came from the correct proposer and
	// that the signature for the data is valid.
	if err := verifier.VerifyProposal(ctx, proposal); err != nil {
		return err
	}

	// Add the proposal to the executor
	store.AddProposal(proposal)

	// pass the proposal on to the state machine to process
	executor.ProcessProposal(proposal.Round)

	return nil
}

func (e *Engine) handleVote(
	ctx context.Context,
	vote *Vote,
	executor *Executor,
	verifier *Verifier,
	store *Store,
) error {
	if err := vote.ValidateForm(); err != nil {
		return err
	}

	// Check that we don't already have the vote
	if store.HasVote(vote) {
		e.logger.Info().
			Uint64("height", vote.Height).
			Uint32("round", vote.Round).
			Msg("vote is already in executor")
		return nil
	}

	// Get the proposal to verify the vote against
	proposalID, _ := store.GetProposal(vote.ProposalRound)
	if proposalID == nil {
		if vote.Height == verifier.Height() {
			store.AddPendingVote(vote)
		}
		// TODO: We may want to consider handling votes at the height directly above
		// the nodes current state.
		return nil
	}

	// verify the vote's signature and other components
	if err := verifier.VerifyVote(vote, proposalID); err != nil {
		return err
	}

	added := store.AddVote(vote)
	if !added {
		// there will have been no state transition so we can exit early
		return nil
	}

	votersWeight := verifier.GetMember(vote.MemberIndex).Weight()
	executor.ProcessVote(vote.Round, vote.ProposalRound, votersWeight, VoteType(vote.Commit))

	return nil
}
