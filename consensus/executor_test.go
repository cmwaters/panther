package consensus_test

import (
	"context"
	"testing"
	"time"

	"github.com/cmwaters/panther/consensus"
	"github.com/stretchr/testify/require"
)

var testCtx = context.Background()

func TestSoloFinalization(t *testing.T) {
	ctx, cancel := context.WithTimeout(testCtx, 10*time.Second)
	defer cancel()
	executor := consensus.NewExecutor(normalVoteFn, proposeInRound(consensus.InitialRound),
		quorumPower(1), totalPower(1), time.Second, time.Second, false)

	errCh := make(chan error)
	go func() {
		errCh <- executor.Run(ctx)
	}()
	select {
	case err := <-errCh:
		require.NoError(t, err)
	case proposalValue := <-executor.Done():
		require.Equal(t, consensus.InitialRound, proposalValue)
	}
}

func TestGroupFinalization(t *testing.T) {
	ctx, cancel := context.WithTimeout(testCtx, 10*time.Second)
	defer cancel()
	executor := consensus.NewExecutor(normalVoteFn, proposeInRound(consensus.InitialRound),
		quorumPower(1), totalPower(1), time.Second, time.Second, false)

	errCh := make(chan error)
	go func() {
		errCh <- executor.Run(ctx)
	}()
	executor.ProcessVote(1, 1, 33, consensus.LOCK)
	executor.ProcessVote(1, 1, 33, consensus.LOCK)
	executor.ProcessVote(1, 1, 33, consensus.COMMIT)
	executor.ProcessVote(1, 1, 33, consensus.COMMIT)
	select {
	case err := <-errCh:
		require.NoError(t, err, executor.Trace().String())
	case proposalValue := <-executor.Done():
		require.Equal(t, consensus.InitialRound, proposalValue)
	}
}

func TestGroupDelayedFinalization(t *testing.T) {
	ctx, cancel := context.WithTimeout(testCtx, 10*time.Second)
	defer cancel()
	executor := setupExecutor()

	errCh := make(chan error)
	go func() {
		errCh <- executor.Run(ctx)
	}()
	// should do nothing because this is for a future
	// round
	executor.ProcessVote(2, 0, 66, consensus.COMMIT)
	// should eventually timeout and COMMIT nil, with the extra
	// voting power moving to round 2 and eventually round 3
	executor.ProcessVote(1, 0, 66, consensus.COMMIT)
	// receives the proposal for round 3 and LOCK votes
	executor.ProcessProposal(2)
	executor.ProcessVote(3, 2, 66, consensus.LOCK)
	// reveives 2f COMMIT votes and sends +1 COMMIT votes
	// eventually finalizing
	executor.ProcessVote(3, 2, 66, consensus.COMMIT)
	select {
	case err := <-errCh:
		require.NoError(t, err)
	case proposalValue := <-executor.Done():
		require.EqualValues(t, 2, proposalValue)
	}
}

func TestTimeoutProposeMoveToNextRound(t *testing.T) {
	ctx, cancel := context.WithTimeout(testCtx, 10*time.Second)
	defer cancel()
	executor := consensus.NewExecutor(normalVoteFn, proposeInRound(consensus.InitialRound+1),
		quorumPower(1), totalPower(1), time.Second, time.Second, false)

	errCh := make(chan error)
	go func() {
		errCh <- executor.Run(ctx)
	}()
	select {
	case err := <-errCh:
		require.NoError(t, err)
	case proposalValue := <-executor.Done():
		require.Equal(t, consensus.InitialRound+1, proposalValue)
	}
}

func proposeInRound(rounds ...uint32) consensus.ProposeFn {
	return func(_ context.Context, round uint32) (bool, error) {
		for _, r := range rounds {
			if r == round {
				return true, nil
			}
		}
		return false, nil
	}
}

func quorumPower(faultyPower uint32) uint32 {
	return 2*faultyPower + 1
}

func totalPower(faultyPower uint32) uint64 {
	return 3*uint64(faultyPower) + 1
}

func setupExecutor() *consensus.Executor {
	return consensus.NewExecutor(normalVoteFn, proposeInRound(), 1, 100, time.Second, time.Second, true)
}

var normalVoteFn = func(_ context.Context, _ uint32, _ uint32, _ bool) error {
	return nil
}
