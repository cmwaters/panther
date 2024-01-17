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

			err = e.gossip.ReportInvalidProposal(ctx, proposal, err)
			if err != nil {
				e.logger.Err(err).Msg("reporting invalid proposal")
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

				err = e.gossip.ReportInvalidVote(ctx, vote, err)
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
	proposalID, err := verifier.VerifyProposal(ctx, proposal)
	if err != nil {
		return err
	}

	// Add the proposal to the executor
	store.AddProposal(proposal, proposalID)

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
	proposalID := store.GetProposalID(vote.ProposalRound)
	if proposalID == nil {
		// we don't have the proposal yet, so we store the vote in pending
		// and process it if the proposal comes
		store.AddPendingVote(vote)
		return nil
	}

	// verify the vote's signature and other components
	if err := verifier.VerifyVote(vote, proposalID); err != nil {
		return err
	}

	store.AddVote(vote)

	votersWeight := verifier.GetMember(vote.MemberIndex).Weight()
	executor.ProcessVote(vote.Round, vote.ProposalRound, votersWeight, VoteType(vote.Commit))

	return nil
}
