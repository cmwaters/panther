package consensus_test

import (
	"fmt"
	"testing"

	"github.com/cmwaters/halo/consensus"
	"github.com/stretchr/testify/require"
)

func TestVoteOnFirstProposal(t *testing.T) {
	tally := consensus.NewTally(100)
	output := tally.Step(consensus.ProposalInput(1))
	require.Equal(t, consensus.LockPhase, tally.Phase())
	require.True(t, output.IsVote())
	proposalRound, voteType := output.GetVoteInfo()
	require.Equal(t, uint32(1), proposalRound)
	require.Equal(t, voteType, consensus.LOCK)
}

func TestDoesNotVoteOnSecondProposal(t *testing.T) {
	tally := consensus.NewTally(100)
	// process receives a proposal from a future round
	output := tally.Step(consensus.ProposalInput(2))
	// does not vote on it
	require.Equal(t, consensus.NoOutput, output)
	// process receives 2f + 1 COMMIT votes
	// for nil proposal and so moves on to the next
	// round, it immediately LOCK votes for the proposal
	// in round 2
	output = tally.Step(consensus.VoteInput(1, 0, 67, consensus.COMMIT))
	require.EqualValues(t, 2, tally.Round())
	require.Equal(t, consensus.VoteOutput(2, consensus.LOCK), output)
}

func TestVoteOnEarliestSeenProposal(t *testing.T) {
	tally := consensus.NewTally(100)
	output := tally.Step(consensus.ProposalInput(2))
	require.Equal(t, consensus.NoOutput, output)
	_ = tally.Step(consensus.ProposalInput(3))
	// seeing proposal at round 3 should not change the earliest proposal
	// seen at round 2
	require.EqualValues(t, 2, tally.ValidProposal())
	output = tally.Step(consensus.VoteInput(1, 0, 67, consensus.COMMIT))
	// 2f + 1 commit nil and we move to the next round, now the tally
	// can vote LOCK for the proposal it saw in round 2
	require.Equal(t, consensus.VoteOutput(2, consensus.LOCK), output)
	// the tally now receives the missing proposal at round 1
	_ = tally.Step(consensus.ProposalInput(1))
	require.EqualValues(t, 1, tally.ValidProposal())
	output = tally.Step(consensus.VoteInput(2, 0, 67, consensus.COMMIT))
	// 2f + 1 COMMIT nil again and the tally progresses to the next round
	// now the lowest valid proposal is at height 1 so it votes that.
	require.Equal(t, consensus.VoteOutput(1, consensus.LOCK), output)
}

func TestRoundProgressions(t *testing.T) {
	tally := consensus.NewTally(100)
	output := tally.Step(consensus.VoteInput(4, 0, 67, consensus.COMMIT))
	require.Equal(t, consensus.NoOutput, output)
	require.Equal(t, consensus.InitialRound, tally.Round())
	output = tally.Step(consensus.VoteInput(3, 0, 67, consensus.COMMIT))
	require.Equal(t, consensus.NoOutput, output)
	require.Equal(t, consensus.InitialRound, tally.Round())
	output = tally.Step(consensus.VoteInput(2, 0, 67, consensus.COMMIT))
	require.Equal(t, consensus.NoOutput, output)
	require.Equal(t, consensus.InitialRound, tally.Round())
	output = tally.Step(consensus.VoteInput(1, 0, 67, consensus.COMMIT))
	require.Equal(t, consensus.ProposalOutput(), output)
	require.EqualValues(t, 5, tally.Round())
}

func TestLockDelay(t *testing.T) {
	tally := consensus.NewTally(100)
	_ = tally.Step(consensus.VoteInput(1, 0, 67, consensus.COMMIT))
	require.EqualValues(t, 2, tally.Round())
	output := tally.Step(consensus.ProposalInput(2))
	require.Equal(t, consensus.VoteOutput(2, consensus.LOCK), output)
	require.Equal(t, consensus.LockPhase, tally.Phase())
	output = tally.Step(consensus.VoteInput(2, 1, 33, consensus.LOCK))
	require.Equal(t, consensus.NoOutput, output)
	output = tally.Step(consensus.VoteInput(2, 2, 34, consensus.LOCK))
	require.Equal(t, consensus.TimeoutOutput(), output)
	require.Equal(t, consensus.LockPhase, tally.Phase())
	output = tally.Step(consensus.TimeoutInput(2, consensus.LockPhase))
	require.Equal(t, consensus.VoteOutput(0, consensus.COMMIT), output)
}

func TestVotesNilAfterTimeout(t *testing.T) {
	tally := consensus.NewTally(100)
	// 2f vote LOCK for a proposal
	_ = tally.Step(consensus.VoteInput(1, 1, 66, consensus.LOCK))
	// Process never sees it and eventually receives a timeout
	output := tally.Step(consensus.TimeoutInput(1, consensus.ProposePhase))
	// should vote COMMIT for Nil
	require.Equal(t, consensus.VoteOutput(consensus.NilProposal, consensus.COMMIT), output)
}

func TestLockingMechanism(t *testing.T) {
	tally := consensus.NewTally(100)
	_ = tally.Step(consensus.ProposalInput(1))
	output := tally.Step(consensus.VoteInput(1, 1, 67, consensus.LOCK))
	require.Equal(t, consensus.VoteOutput(1, consensus.COMMIT), output)
	require.EqualValues(t, 1, tally.LockedProposal())

	// 2f + 1 now vote COMMIT for nil, the tally progresses to the next
	// round and immediately votes COMMIT for the proposal it is locked on
	output = tally.Step(consensus.VoteInput(1, 0, 67, consensus.COMMIT))
	require.Equal(t, consensus.VoteOutput(1, consensus.COMMIT), output)
	require.EqualValues(t, 2, tally.Round())

	// 2f + 1 now lock on a different value to the one the process is
	// currently locked on. The process should now switch locks. It was
	// part of a previous minority. However the process should not vote
	output = tally.Step(consensus.VoteInput(2, 2, 67, consensus.LOCK))
	require.Equal(t, consensus.NoOutput, output)
	require.EqualValues(t, 2, tally.LockedProposal())

	// 2f + 1 again COMMIT nil. The process moves into round 3. It should
	// now vote commit for proposal 2f
	output = tally.Step(consensus.VoteInput(2, 0, 67, consensus.COMMIT))
	require.Equal(t, consensus.VoteOutput(2, consensus.COMMIT), output)

	output = tally.Step(consensus.VoteInput(3, 3, 67, consensus.COMMIT))
	require.Equal(t, consensus.FinalizedOutput(3, 3), output)
}

func TestIgnoreTimeoutWhenProgressingToNextPhase(t *testing.T) {
	tally := consensus.NewTally(100)
	output := tally.Step(consensus.ProposalInput(1))
	require.Equal(t, consensus.VoteOutput(1, consensus.LOCK), output)
	// A proposal timeout is received, but the tally has already moved
	// to the LOCK phase and so ignores it
	output = tally.Step(consensus.TimeoutInput(1, consensus.ProposePhase))
	require.Equal(t, consensus.NoOutput, output)
	require.EqualValues(t, consensus.LockPhase, tally.Phase())
	output = tally.Step(consensus.VoteInput(1, 1, 67, consensus.LOCK))
	require.Equal(t, consensus.VoteOutput(1, consensus.COMMIT), output)
	// A lock delay timeout is triggered, but the tally has already
	// moved to the COMMIT phase and so ignores it
	output = tally.Step(consensus.TimeoutInput(1, consensus.LockPhase))
	require.Equal(t, consensus.NoOutput, output)
}

func TestTallyingQuorum(t *testing.T) {
	testCases := []uint32{1, 3, 33, 10000}
	for _, faultyPower := range testCases {
		t.Run(fmt.Sprintf("%d", faultyPower), func(t *testing.T) {
			quorum := int(quorumPower(faultyPower))
			tally := consensus.NewTally(totalPower(faultyPower))
			_ = tally.Step(consensus.ProposalInput(1))
			for i := 0; i < quorum-1; i++ {
				output := tally.Step(consensus.VoteInput(1, 1, 1, consensus.LOCK))
				require.Equal(t, consensus.NoOutput, output)
			}
			for i := 0; i < quorum-1; i++ {
				output := tally.Step(consensus.VoteInput(1, 2, 1, consensus.LOCK))
				if i == 0 {
					require.Equal(t, consensus.TimeoutOutput(), output)
				} else {
					require.Equal(t, consensus.NoOutput, output)
				}
			}
			require.Equal(t, consensus.LockPhase, tally.Phase())
			output := tally.Step(consensus.TimeoutInput(1, consensus.LockPhase))
			require.Equal(t, consensus.VoteOutput(consensus.NilProposal, consensus.COMMIT), output)
			for i := 0; i < quorum-1; i++ {
				output := tally.Step(consensus.VoteInput(1, 1, 1, consensus.COMMIT))
				require.Equal(t, consensus.CommitPhase, tally.Phase())
				require.Equal(t, consensus.NoOutput, output)
			}
			output = tally.Step(consensus.VoteInput(1, 1, 1, consensus.COMMIT))
			require.Equal(t, consensus.FinalizedOutput(1, 1), output)
		})
	}
}
