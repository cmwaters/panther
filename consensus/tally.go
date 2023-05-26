package consensus

import (
	"context"
	"time"
)

const (
	ProposeStep = iota + 1
	LockStep
	CommitStep

	// We start at round 1, round 0 is the unset/null value
	InitialRound = 1
	NullProposal = 0
)

type (
	// Outputs:
	// Tally is given hooks to vote or propose upon a state transition
	VoteFn    func(context.Context, uint32, uint32, bool) error
	ProposeFn func(context.Context, uint32) (bool, error)
)

// Tally is the core struct responsible for implementing the consensus protocol. It can be viewed as
// a single threaded Mealy state machine. Each incoming proposal, vote or timeout are the inputs. Each
// input can produce a state transition and a set of outputs: Either to propose, vote or set a timeout.
// This is all captured in the `Run` method.
type Tally struct {
	// round and step represent the state of the protocol. Each field is monotonically increasing.
	round uint32
	step  uint8
	// roundState captures the votes gathered at each round and for what proposal
	roundState map[uint32]*roundState

	// lockedValue signals a possible prior value that the protocol has locked on i.e. has voted COMMIT
	// for that value after seeing 2f+1 LOCK votes for that value.
	lockedValue, lockedRound uint32

	validValue, validRound uint32
	

	// totalVotingPower is the total voting power of the group. Multiplied by the quorum fraction, provides
	// the minimum voting power required to reach a quorum and invoke a state transition
	totalVotingPower uint64

	// selfWeight is the voting power of the node itself
	selfWeight uint32

	// proposalTimeout is the timeout for a proposal to be received in a round before being deemed invalid.
	// If the timeout is reached, the protocol will vote nil for the round if it is not already locked on
	// another value
	proposalTimeout time.Duration

	// Upon reaching 2f + 1 LOCK votes for any proposal, the protocol will wait for as long as
	// "lockDelay", for a quorum of a specific value to manifest itself. If so, the protocol will
	// lock and vote to COMMIT that value.
	//
	// A lock delay of 0, will mean that unless the first 2f + 1 in votes all LOCK a value, the
	// process itself, will COMMIT nil for that round.
	lockDelay time.Duration

	// inputCh serializes all forms of input: timeouts, proposals and votes to a single buffered channel
	inputCh chan input

	// voteFn and proposeFn are hooks to the consensus engine to vote or propose upon a state transition
	voteFn    VoteFn
	proposeFn ProposeFn

	// doneCh is a channel that is closed when the protocol has decided on a value, it returns the round
	// of the proposal that was decided
	doneCh chan uint32

	// errCh collects
	errCh chan error
}

func NewTally(
	voteFn VoteFn,
	proposeFn ProposeFn,
	selfWeight uint32,
	totalVotingPower uint64,
	proposalTimeout,
	lockDelay time.Duration,
) *Tally {
	return &Tally{
		doneCh:           make(chan uint32), // done should only happen once (does not need to be buffered)
		inputCh:          make(chan input, 100),
		voteFn:           voteFn,
		proposeFn:        proposeFn,
		selfWeight:       selfWeight,
		totalVotingPower: totalVotingPower,
		round:            InitialRound,
		step:             ProposeStep,
		roundState:       make(map[uint32]*roundState),
		proposalTimeout:  proposalTimeout,
		lockDelay:        lockDelay,
	}
}

// Run is the main loop of the Tally. It is responsible for processing all serialized inputs:
// timeouts, proposals and votes, performing state transitions and producing outputs: votes,
// proposals and timeouts all according the logic of the consensus protocol.
func (t *Tally) Run(ctx context.Context) error {
	t.proposeFn(ctx, t.round)
	t.scheduleProposalTimeout(t.proposalTimeout, t.round)
	for {
		// pull in next input from the channel
		var input input
		select {
		case <-ctx.Done():
			return ctx.Err()
		// exit if any of the outputs failed to execute
		case err := <-t.errCh:
			return err
		case input = <-t.inputCh:
		}
		switch {
		// handle timeout
		case input.timeout != nil:
			// check if the timeout is for the current round
			if input.timeout.round != t.round {
				// the process has since moved to a later round
				continue
			}

			// check if the timeout is for the current step
			if input.timeout.step != t.step {
				// the process has since moved to a later step
				continue
			}

			// For both timeouts (ProposeStep and LockStep), the process will
			// COMMIT nil for the round
			//
			// If ProposeStep: the process has not received a proposal within the timeout
			// while expecting a proposal (i.e. the process is not yet locked on a value).
			//
			// If LockStep: the process has waited for more LOCK votes to arrive but not
			// enough have been received to reach quorum and LOCK on a value.
			t.voteCommit(ctx, t.round, NullProposal)

		// handle proposals
		case input.proposal != nil:
			// check if we are locked on a value
			if t.lockedValue == 0 {

			}

		// handle votes
		case input.vote != nil:
			vote := input.vote
			roundState := t.inRound(vote.round)
			roundState.addVote(vote.proposalRound, vote.weight, vote.commit)

			// handle commit vote
			if vote.commit {
				if roundState.hasQuorum(vote.proposalRound, true) {
					// the process has received 2f + 1 COMMIT votes for a proposal
					// the value has been finalized
					t.doneCh <- vote.proposalRound
					return nil
				}

				if roundState.hasQuorumVoted(true) && t.round == vote.round {
					// the process has received 2f + 1 COMMIT votes for any proposal in its current
					// round. The process progresses to the next round
					t.round++
					switch {
					case t.isLocked():
						t.voteCommit(ctx, t.round, t.lockedValue)
						t.step = CommitStep
					case t.seenValidProposal():
						t.voteLock(ctx, t.round, t.validValue)
						t.step = LockStep
					default:
						t.propose(ctx, t.round)
						t.scheduleProposalTimeout(t.proposalTimeout, t.round)
						t.step = ProposeStep
					}
					continue
				}
			}

			// handle lock vote
			if roundState.hasQuorum(vote.proposalRound, false) {
				// the process has received 2f + 1 LOCK votes for a proposal
				if t.lockedRound < vote.round {
					// the process either hasn't locked on a value or a quorum has voted
					// in a later round for another proposal. In either case we set the lock
					// on that value
					t.lockedValue = vote.proposalRound
					t.lockedRound = vote.round
					t.voteCommit(ctx, t.round, t.lockedValue)
					t.step = CommitStep
				}
			}

		default:
			panic("nil input")
		}
	}
}

// ProcessVote queues a vote to be processed by the Tally. It assumes that the vote is valid and unique.
// It assumes that the value that the vote is voting for i.e. the round exists.
func (t *Tally) ProcessVote(round, proposalRound, weight uint32, commit bool) {
	t.inputCh <- voteInput(round, proposalRound, weight, commit)
}

// ProcessProposal queues a proposal to be processed by the Tally. It assumes that the proposal is valid
// and unique.
func (t *Tally) ProcessProposal(round uint32) {
	t.inputCh <- proposalInput(round)
}

func (t *Tally) processTimeout(round uint32, step uint8) {
	t.inputCh <- timeoutInput(round, step)
}

func (t *Tally) Done() <-chan uint32 {
	return t.doneCh
}

func (t *Tally) scheduleProposalTimeout(period time.Duration, round uint32) {
	go func() {
		time.AfterFunc(period, func() {
			t.processTimeout(round, ProposeStep)
		})
	}()
}

func (t *Tally) scheduleLockDelay(period time.Duration, round uint32) {
	time.AfterFunc(period, func() {
		t.processTimeout(round, LockStep)
	})
}

func (t *Tally) propose(ctx context.Context, round uint32) {
	go func () {
		proposed, err := t.proposeFn(ctx, round)
		if err != nil {
			select {
			case <-ctx.Done():
			default:
				t.errCh <- err
			}
		}
		if proposed {
			// queue the nodes own proposal to be processed
			t.ProcessProposal(round)
		}
	}()
}

func (t *Tally) voteLock(ctx context.Context, round, proposalRound uint32) {
	go func() {
		if err := t.voteFn(ctx, round, proposalRound, false); err != nil {
			select { // make sure to not block if the context is cancelled
			case <-ctx.Done():
			case t.errCh <- err:
			}
			return
		}
		t.ProcessVote(round, proposalRound, t.selfWeight, false)
	}()
}

func (t *Tally) voteCommit(ctx context.Context, round, proposalRound uint32) {
	go func() {
		if err := t.voteFn(ctx, round, proposalRound, true); err != nil {
			select { // make sure to not block if the context is cancelled
			case <-ctx.Done():
			case t.errCh <- err:
			}
			return
		}
		t.ProcessVote(round, proposalRound, t.selfWeight, true)
	}()
}

func (t *Tally) isLocked() bool {
	return t.lockedValue != 0
}

func (t *Tally) seenValidProposal() bool {
	return t.validValue != 0
}

func (t *Tally) hasProposal(round uint32) bool {
	return t.inRound(round).hasProposal
}

func (t *Tally) inRound(round uint32) *roundState {
	rs, ok := t.roundState[round]
	if !ok {
		rs = newRoundState(t.totalVotingPower)
		t.roundState[round] = rs
	}
	return rs
}

type input struct {
	proposal *proposalInfo
	vote     *voteInfo
	timeout  *timeoutInfo
}

type roundState struct {
	hasProposal      bool
	lockVotes        map[uint32]uint64
	totalLockVotingPower   uint64
	commitVotes      map[uint32]uint64
	totalCommitVotingPower uint64
	totalVotingPower uint64
}

func newRoundState(totalVotingPower uint64) *roundState {
	return &roundState{
		lockVotes:   make(map[uint32]uint64),
		commitVotes: make(map[uint32]uint64),
	}
}

func (rs *roundState) addVote(proposalRound, weight uint32, commit bool) {
	if commit {
		rs.commitVotes[proposalRound] += uint64(weight)
		rs.totalCommitVotingPower += uint64(weight)
	} else {
		rs.lockVotes[proposalRound] += uint64(weight)
		rs.totalLockVotingPower += uint64(weight)
	}
}

func (rs *roundState) hasQuorum(proposalRound uint32, commit bool) bool {
	if commit {
		return rs.commitVotes[proposalRound] * 3 >= rs.totalCommitVotingPower * 2
	}
	return rs.lockVotes[proposalRound] * 3 >= rs.totalLockVotingPower * 2
}

func (rs *roundState) hasQuorumVoted(commit bool) bool {
	if commit {
		return rs.totalCommitVotingPower * 3 >= rs.totalVotingPower * 2
	}
	return rs.totalLockVotingPower * 3 >= rs.totalVotingPower * 2
}


type proposalInfo struct {
	round uint32
}

type voteInfo struct {
	round, proposalRound, weight uint32
	commit bool
}

type timeoutInfo struct {
	round uint32
	step  uint8
}

func voteInput(round, proposalRound, weight uint32, commit bool) input {
	return input{
		vote: &voteInfo{
			round:         round,
			proposalRound: proposalRound,
			weight:        weight,
			commit:        commit,
		},
	}
}

func proposalInput(round uint32) input {
	return input{
		proposal: &proposalInfo{round},
	}
}

func timeoutInput(round uint32, step uint8) input {
	return input{
		timeout: &timeoutInfo{round, step},
	}
}
