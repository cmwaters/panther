package consensus

import "fmt"

const (
	// Tally has three phases within a round:
	//
	// ProposePhase: enter at the start of a round if in that round, the
	// process has not yet received a valid proposal. Exit once the
	// process has voted LOCK on a proposal or after the ProposalTimeout
	ProposePhase uint8 = iota + 1
	// LockPhase: enter after voting LOCK on a proposal. Exit once
	// the process has received 2f + 1 LOCK votes for any value
	// and after the LockDelay period
	LockPhase
	// CommitPhase: enter after voting COMMIT in a round. Exit once
	// the process has received 2f + 1 LOCK votes for any value. There
	// is no timeout with this phase
	CommitPhase

	// We start at round 1, round 0 is the unset/null value
	InitialRound uint32 = 1
	NilProposal  uint32 = 0

	LOCK   VoteType = false
	COMMIT VoteType = true
)

type VoteType bool

// Tally is the core struct responsible for implementing the consensus protocol. It can be viewed as
// a single threaded Mealy state machine. Each incoming proposal, vote or timeout are the inputs. Each
// input can produce a state transition and a set of outputs: Either to propose, vote or set a timeout.
// This is all captured in the `Run` method which iteratively steps through the protocol until a value
// is finalized.
//
// Inputs can be staged through the `ProcessVote` and `ProcessProposal` methods. Timeouts are created
// and handled internally although the timeout durations are specified by the caller in the constructor.
// It is assumed that votes for a proposal come strictly after the proposal itself.
type Tally struct {
	// round and phase represent the state of the protocol. Each field is monotonically increasing.
	round uint32
	phase uint8
	// roundState captures the votes gathered at each round and for what proposal
	roundState map[uint32]*roundState

	// lockedValue signals a possible prior value that the protocol has locked on i.e. has voted COMMIT
	// for that value after seeing 2f+1 LOCK votes for that value.
	lockedProposal, lockedRound uint32

	// validValue tracks the lowest round proposal that the process has seen. If a process is not
	// locked on any proposal, it will vote LOCK for "validValue" in ProposePhase
	validProposal uint32

	// totalVotingPower is the total voting power of the group. Multiplied by the quorum fraction, provides
	// the minimum voting power required to reach a quorum and invoke a state transition
	totalVotingPower uint64

	// lastLockDelayRound is the last round that the
	// timeout was triggered. It's used simply to prevent
	// the timeout being called multiple times in a round
	lastLockDelayRound uint32
}

func NewTally(totalVotingPower uint64) *Tally {
	return &Tally{
		totalVotingPower: totalVotingPower,
		round:            InitialRound,
		phase:            ProposePhase,
		roundState:       make(map[uint32]*roundState),
	}
}

// Step defines a discrete step in handling a single input (proposal, vote or timeout) according
// to the logic of the consensus protocol.
func (t *Tally) Step(input Input) Output {
	switch {
	// handle proposals
	case input.proposal != nil:
		if !t.seenValidProposal() && t.phase == ProposePhase && input.proposal.round <= t.round {
			// this is the first valid proposal that the process has
			// seen. It is in ProposePhase so it has not yet timed out.
			// It it also not a proposal from a future round.
			// Vote LOCK on the proposal
			t.phase = LockPhase
			return VoteOutput(input.proposal.round, LOCK)
		}

		// check if we can update the validValue
		if t.validProposal == 0 || input.proposal.round < t.validProposal {
			t.validProposal = input.proposal.round
		}
		// there is nothing more to do here. Other forms of voting happen
		// while processing either a timeout or a vote

	// handle votes
	case input.vote != nil:
		vote := input.vote
		roundState := t.inRound(vote.round)
		roundState.addVote(vote.proposalRound, vote.weight, vote.voteType)

		// handle commit vote
		if vote.voteType == COMMIT {
			if vote.proposalRound != NilProposal && roundState.hasQuorum(vote.proposalRound, COMMIT) {
				// the process has received 2f + 1 COMMIT votes for a proposal
				// the value has been finalized
				return FinalizedOutput(vote.proposalRound)
			}

			if t.round == vote.round {
				// If the process has received 2f + 1 COMMIT votes for ANY proposal in its current
				// round. The process progresses to the next round (Note it may go through multiple
				// rounds)
				for roundState.hasQuorumVoted(COMMIT) {
					t.round++
					roundState = t.inRound(t.round)
				}
				if t.round > vote.round {
					// the process has progressed to a new round
					switch {
					case t.isLocked():
						// the process is already locked on a value i.e. it observed 2f + 1 LOCK votes.
						// Two correct processes may lock on two different proposals but this mechanism
						// guarantees that only one proposal is locked on by 2f + 1 processes and thus
						// less than f + 1 correct process will lock on any other proposal.
						// The process can immediately vote COMMIT again. If this process is part of the
						// minority it will never receive 2f + 1 COMMIT votes (even if f are faulty) and
						// will eventually observe 2f + 1 LOCK or COMMIT votes for a different proposal.
						t.phase = CommitPhase
						return VoteOutput(t.lockedProposal, COMMIT)
					case t.seenValidProposal():
						// The process is not locked on a proposal, but it has already seen a valid
						// proposal. It does not need to propose a counter value but instead votes
						// LOCK for the validProposal
						t.phase = LockPhase
						return VoteOutput(t.validProposal, LOCK)
					default:
						// The process has not received a valid proposal. It begins the new round by
						// proposing a new value if it is the proposer and setting a timeout with
						// which to receive a proposal.
						t.phase = ProposePhase
						return ProposalOutput()
					}
				}
			}
		}

		// handle lock vote
		if roundState.hasQuorum(vote.proposalRound, LOCK) {
			// the process has received 2f + 1 LOCK votes for a proposal
			if t.lockedRound < vote.round {
				// the process either hasn't locked on a value or a quorum has voted
				// in a later round for another proposal. In either case we set the lock
				// on that value
				t.lockedProposal = vote.proposalRound
				t.lockedRound = vote.round

				// If the process is at the LockPhase, then we have not yet voted COMMIT
				// for a value. Now that 2f + 1 are locked on a value, the process can
				// vote COMMIT for that value. (Note it's perfectly valid to vote LOCK
				// for one value and then vote COMMIT for a different value)
				if t.phase == LockPhase {
					t.phase = CommitPhase
					return VoteOutput(t.lockedProposal, COMMIT)
				}

				// Given the assumption that votes for a proposal must come strictly
				// after the proposal, it is impossible that the process is in the
				// propose phase.

				// If the process were in the commit phase, it means the process has
				// already voted COMMIT, so we do nothing until the next round
				return NoOutput
			}
		}

		if roundState.hasQuorumVoted(LOCK) && t.round == vote.round && t.round > t.lastLockDelayRound {
			// The process schedules a timeout here before deciding what its `COMMIT` vote
			// will be. Without the timeout, unless the first 2f + 1 LOCK votes were all for
			// a value, the process would COMMIT vote nil. By setting some delay for later
			// votes to arrive, we improve the change that there is 2f + 1 LOCK for a value
			// and that the process can immediately COMMIT that.
			t.lastLockDelayRound = t.round
			return TimeoutOutput()
		}

	// handle timeouts
	case input.timeout != nil:
		// check if the timeout is for the current round
		if input.timeout.round != t.round {
			// the process has since moved to a later round
			return NoOutput
		}

		// check if the timeout is for the current phase
		if input.timeout.phase != t.phase {
			// the process has since moved to a later phase
			return NoOutput
		}

		// For both timeouts (ProposePhase and LockPhase), the process will
		// COMMIT nil for the round
		//
		// If ProposePhase: the process has not received a proposal within the timeout
		// while expecting a proposal (i.e. the process is not yet locked on a value).
		//
		// If LockPhase: the process has waited for more LOCK votes to arrive but not
		// enough have been received to reach quorum and LOCK on a value.
		t.phase = CommitPhase
		return VoteOutput(NilProposal, COMMIT)

	default:
		panic("nil input")
	}

	return NoOutput
}

// Round returns the current round of the Tally.
// Do not use concurrently with `Run`
func (t *Tally) Round() uint32 {
	return t.round
}

// Phase returns the current phase of the Tally.
// Do not use concurrently with `Run`
func (t *Tally) Phase() uint8 {
	return t.phase
}

// ValidProposal returns the process lowest valid proposal.
// Do not use concurrently with `Run`
func (t *Tally) ValidProposal() uint32 {
	return t.validProposal
}

// LockedProposal returns the proposal value that the process is locked on
// Do not use concurrently with `Run“
func (t *Tally) LockedProposal() uint32 {
	return t.lockedProposal
}

func (t *Tally) isLocked() bool {
	return t.lockedProposal != 0
}

func (t *Tally) seenValidProposal() bool {
	return t.validProposal != 0
}

func (t *Tally) inRound(round uint32) *roundState {
	rs, ok := t.roundState[round]
	if !ok {
		rs = newRoundState(t.totalVotingPower)
		t.roundState[round] = rs
	}
	return rs
}

type roundState struct {
	hasProposal            bool
	lockVotes              map[uint32]uint64
	totalLockVotingPower   uint64
	commitVotes            map[uint32]uint64
	totalCommitVotingPower uint64
	totalVotingPower       uint64
}

func newRoundState(totalVotingPower uint64) *roundState {
	return &roundState{
		lockVotes:        make(map[uint32]uint64),
		commitVotes:      make(map[uint32]uint64),
		totalVotingPower: totalVotingPower,
	}
}

func (rs *roundState) addVote(proposalRound, weight uint32, voteType VoteType) {
	switch voteType {
	case COMMIT:
		rs.commitVotes[proposalRound] += uint64(weight)
		rs.totalCommitVotingPower += uint64(weight)
		// nil commits are counted also as nil lock votes
		// a process only needs to vote nil once
		if proposalRound == NilProposal {
			rs.lockVotes[proposalRound] += uint64(weight)
			rs.totalLockVotingPower += uint64(weight)
		}
	case LOCK:
		rs.lockVotes[proposalRound] += uint64(weight)
		rs.totalLockVotingPower += uint64(weight)
	}
}

func (rs *roundState) hasQuorum(proposalRound uint32, voteType VoteType) bool {
	if voteType == COMMIT {
		return rs.commitVotes[proposalRound]*3 > rs.totalVotingPower*2
	}
	return rs.lockVotes[proposalRound]*3 > rs.totalVotingPower*2
}

func (rs *roundState) hasQuorumVoted(voteType VoteType) bool {
	if voteType == COMMIT {
		return rs.totalCommitVotingPower*3 > rs.totalVotingPower*2
	}
	return rs.totalLockVotingPower*3 > rs.totalVotingPower*2
}

type (
	Input struct {
		proposal *proposalEvent
		vote     *voteEvent
		timeout  *timeoutEvent
	}

	// Output and Input are the same
	Output struct {
		proposal               bool
		timeout                bool
		vote                   bool
		voteType               VoteType
		proposalRound          uint32
		finalizedProposalRound uint32
	}
)

func (i Input) String() string {
	switch {
	case i.proposal != nil:
		return fmt.Sprintf("proposal{%d}", i.proposal.round)
	case i.vote != nil:
		voteType := "lock"
		if i.vote.voteType == COMMIT {
			voteType = "commit"
		}
		return fmt.Sprintf("vote{%s for %d @ %d:%d}", voteType, i.vote.proposalRound, i.vote.round, i.vote.weight)
	case i.timeout != nil:
		return fmt.Sprintf("timeout{%d, %d}", i.timeout.round, i.timeout.phase)
	default:
		return "none"
	}
}

var NoOutput = Output{}

func VoteOutput(proposalRound uint32, voteType VoteType) Output {
	return Output{
		vote:          true,
		voteType:      voteType,
		proposalRound: proposalRound,
	}
}

func TimeoutOutput() Output {
	return Output{timeout: true}
}

func ProposalOutput() Output {
	return Output{proposal: true}
}

func FinalizedOutput(proposalRound uint32) Output {
	return Output{finalizedProposalRound: proposalRound}
}

func (o Output) IsNone() bool {
	return o.proposal == false && o.vote == false && o.timeout == false
}

func (o Output) IsProposal() bool {
	return o.proposal
}

func (o Output) IsVote() bool {
	return o.vote
}

func (o Output) IsTimeout() bool {
	return o.timeout
}

func (o Output) HasFinalized() bool {
	return o.finalizedProposalRound != 0
}

func (o Output) GetVoteInfo() (proposalRound uint32, voteType VoteType) {
	if !o.vote {
		return
	}
	return o.proposalRound, o.voteType
}

func (o Output) GetFinalizedProposalRound() uint32 {
	return o.finalizedProposalRound
}

func (o Output) String() string {
	switch {
	case o.proposal:
		return "proposal"
	case o.vote:
		voteType := "lock"
		if o.voteType == COMMIT {
			voteType = "commit"
		}
		return fmt.Sprintf("vote{%d, %s}", o.proposalRound, voteType)
	case o.timeout:
		return "timeout"
	case o.finalizedProposalRound != 0:
		return fmt.Sprintf("finalized{%d}", o.finalizedProposalRound)
	default:
		return "none"
	}
}

type proposalEvent struct {
	round uint32
}

type voteEvent struct {
	round, proposalRound, weight uint32
	voteType                     VoteType
}

type timeoutEvent struct {
	round uint32
	phase uint8
}

func VoteInput(round, proposalRound, weight uint32, voteType VoteType) Input {
	return Input{
		vote: &voteEvent{
			round:         round,
			proposalRound: proposalRound,
			weight:        weight,
			voteType:      voteType,
		},
	}
}

func ProposalInput(round uint32) Input {
	return Input{
		proposal: &proposalEvent{round},
	}
}

func TimeoutInput(round uint32, phase uint8) Input {
	return Input{
		timeout: &timeoutEvent{round, phase},
	}
}
