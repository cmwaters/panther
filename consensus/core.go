package consensus

import (
	"context"
	"errors"
)

type DecideFn func(ctx context.Context, height uint64, round uint32) error

// timeoutDecide is called when the protocol must make a decision, triggered from the
// timeout after the synchronous period. The consensus engine uses the tally of votes
// and proposals to decide what value it will signal for. All correct processes will
// eventually converge on that same value.
func (e *Engine) timeoutDecide(ctx context.Context, height uint64, round uint32) error {
	switch {
	case !e.tally.HasProposal(round):
	}
	return nil
}

// handleQuorum handles the scenario when a majority of votes is reached in a specific round. This
// can be called multiple times within a round. It can also be safely called even if the process has
// Already decided. Under specified conditions, it can lead to the committing of a value or the 
// progression into a new round
func (e *Engine) handleQuorum(ctx context.Context, round uint32, state *State, proposal *Proposal) error {
	switch {
	// Has a quorum of members voted to commit a value?
	case e.tally.HasQuorumCommit(round):
		// Last necessary commit vote has been received. We have reached a quorum
		// for the given proposal
		if err := e.commit(ctx, proposal); err != nil {
			return unrecoverable(err)
		}
		return nil

	// Has a quorum of members voted to signal for a value?
	case e.tally.HasQuorumSignal(round):
		// Retrieve the relevant proposal. By design, this proposal must exist because
		// we only process votes after receiving the proposal.
		digest, _ := e.tally.GetProposal(round)
		err := e.executor.tryDecide(ctx, state.Height, state.Round, func(ctx context.Context, height uint64, round uint32) error {
			return e.broadcastVote(ctx, height, round, proposal.Round, digest, Vote_COMMIT)
		})
		if err != nil {
			return unrecoverable(err)
		}
	}

	// Is there a quorum of members that have signalled for ANY value in the round that equals
	// our latest known round? If so, we can progress to the next round.
	if round == state.Round {
		if e.executor.nextRound(ctx, state.Round+1, e.timeoutDecide) {
			if e.signer != nil && e.executor.isProposer(e.ourPubkey) {
				// TODO: we need to check if the proposer has already received a valid
				// proposal in which the node may simply vote for it as a way of reproposing.
				return e.propose(ctx, state.Height, state.Round+1, state.Round+1)
			}
		}
	}

	// Do nothing
	return nil
}

// commit commits a value that has reached quorum, applying the data to the
// state machine and advancing to the next height. It then runs the logic
// for proposing and sets the respective timeout.
func (e *Engine) commit(ctx context.Context, proposal *Proposal) error {
	// Use the executor to finalize the value, posting it to the application
	// and using the returned members and params to advance to the next height
	if err := e.executor.finalize(ctx, proposal, e.app, e.tally, e.timeoutDecide); err != nil {
		if errors.Is(err, ErrApplicationShutdown) {
			return err
		}
		return unrecoverable(err)
	}
	// Check if we are the proposer for the first round of the new height.
	if e.signer != nil && e.executor.isProposer(e.ourPubkey) {
		return e.propose(ctx, proposal.Height+1, 0, 0)
	}
	return nil
}
