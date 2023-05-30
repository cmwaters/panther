package consensus

import (
	"context"
	"fmt"
	"time"
)

type (
	// Outputs:
	// Tally is given hooks to vote or propose upon a state transition
	VoteFn    func(context.Context, uint32, uint32, bool) error
	ProposeFn func(context.Context, uint32) (bool, error)
)

// Executor executes
type Executor struct {
	tally *Tally

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
	inputCh chan Input

	// voteFn and proposeFn are hooks to the consensus engine to vote or propose upon a state transition
	voteFn    VoteFn
	proposeFn ProposeFn

	// doneCh is a channel that is closed when the protocol has decided on a value, it returns the round
	// of the proposal that was decided
	doneCh chan uint32

	// errCh collects errors from execution
	errCh chan error

	// trace is an optional object for observability used to track every
	// input and output
	trace *Trace
}

func NewExecutor(
	voteFn VoteFn,
	proposeFn ProposeFn,
	selfWeight uint32,
	totalVotingPower uint64,
	proposalTimeout,
	lockDelay time.Duration,
	enableTracing bool,
) *Executor {
	var trace *Trace
	if enableTracing {
		trace = newTrace()
	}
	return &Executor{
		tally:           NewTally(totalVotingPower),
		doneCh:          make(chan uint32), // done should only happen once (does not need to be buffered)
		inputCh:         make(chan Input, 100),
		voteFn:          voteFn,
		proposeFn:       proposeFn,
		selfWeight:      selfWeight,
		proposalTimeout: proposalTimeout,
		lockDelay:       lockDelay,
		trace:           trace,
	}
}

// Run is the main loop of the Tally. It is responsible for processing all serialized inputs:
// timeouts, proposals and votes, performing state transitions and producing outputs: votes,
// proposals and timeouts all according the logic of the consensus protocol.
func (e *Executor) Run(ctx context.Context) error {
	if e.tally.Phase() == ProposePhase {
		// upon starting, the Tally signals to propose a value if the process is the
		// current proposer. The Tally also begins the first timeout
		e.propose(ctx, e.tally.Round())
	}
	for {
		select {
		// catch context cancellation
		case <-ctx.Done():
			return ctx.Err()

		// return an error if any of the outputs failed to execute
		case err := <-e.errCh:
			return err

		// return once consensus has finalized
		case <-e.Done():
			return nil

		// pull the next input from the channel
		case input := <-e.inputCh:
			// apply the input to the state machine
			// and receive the output
			output := e.tally.Step(input)

			if e.trace != nil {
				e.trace.Add(input, output)
			}

			// execute the output
			switch {
			case output.HasFinalized():
				e.doneCh <- output.GetFinalizedProposalRound()
			case output.IsNone():
				continue
			case output.IsProposal():
				e.propose(ctx, e.tally.Round())
			case output.IsVote():
				proposalRound, commit := output.GetVoteInfo()
				e.vote(ctx, e.tally.Round(), proposalRound, commit)
			case output.IsTimeout():
				e.scheduleLockDelay(e.lockDelay, e.tally.Round())
			}
		}

	}
}

// Done returns a channel that will receive the proposal value upon finalization
func (e *Executor) Done() <-chan uint32 {
	return e.doneCh
}

// ProcessVote queues a vote to be processed by the tally. It assumes that the vote is valid and unique.
// It assumes that the value that the vote is voting for i.e. the round exists.
func (e *Executor) ProcessVote(round, proposalRound, weight uint32, voteType VoteType) {
	e.inputCh <- VoteInput(round, proposalRound, weight, voteType)
}

// ProcessProposal queues a proposal to be processed by the tally. It assumes that the proposal is valid
// and unique.
func (e *Executor) ProcessProposal(round uint32) {
	e.inputCh <- ProposalInput(round)
}

// Trace returns the underlying trace. This method is not concurrently safe
func (e Executor) Trace() *Trace {
	return e.trace
}

// processTimeout inserts a timeout to the inputCh
func (e *Executor) processTimeout(round uint32, phase uint8) {
	e.inputCh <- TimeoutInput(round, phase)
}

// scheduleProposalTimeout triggers a timeout after the provided "period".
func (e *Executor) scheduleProposalTimeout(period time.Duration, round uint32) {
	go func() {
		time.AfterFunc(period, func() {
			e.processTimeout(round, ProposePhase)
		})
	}()
}

// scheduleLockDelay triggers a timeout after the provided "period"
func (e *Executor) scheduleLockDelay(period time.Duration, round uint32) {
	time.AfterFunc(period, func() {
		e.processTimeout(round, LockPhase)
	})
}

func (e *Executor) propose(ctx context.Context, round uint32) {
	go func() {
		if e.proposeFn == nil {
			return
		}
		proposed, err := e.proposeFn(ctx, round)
		if err != nil {
			select {
			case <-ctx.Done():
			default:
				e.errCh <- err
			}
		}
		if proposed {
			// queue the nodes own proposal to be processed
			e.ProcessProposal(round)
		}
	}()
	e.scheduleProposalTimeout(e.proposalTimeout, round)
}

func (e *Executor) vote(ctx context.Context, round, proposalRound uint32, voteType VoteType) {
	go func() {
		if e.voteFn == nil {
			return
		}
		if err := e.voteFn(ctx, round, proposalRound, bool(voteType)); err != nil {
			select { // make sure to not block if the context is cancelled
			case <-ctx.Done():
			case e.errCh <- err:
			}
			return
		}
		e.ProcessVote(round, proposalRound, e.selfWeight, voteType)
	}()
}

type Trace struct {
	Input  []Input
	Output []Output
}

func newTrace() *Trace {
	return &Trace{
		Input:  make([]Input, 0),
		Output: make([]Output, 0),
	}
}

func (t *Trace) Add(input Input, output Output) {
	t.Input = append(t.Input, input)
	t.Output = append(t.Output, output)
}

func (t *Trace) String() string {
	str := ""
	for i := 0; i < len(t.Input); i++ {
		str += fmt.Sprintf("%s -> %s\n", t.Input[i].String(), t.Output[i].String())
	}
	return str
}
