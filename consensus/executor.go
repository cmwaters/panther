package consensus

import (
	"bytes"
	"context"
	"sync"
	"sync/atomic"
	"time"
)


// executor is a struct responsible for controlling execution, acting like a semaphore
// to ensure that the properties of the consensus protocol are upheld:
// - Nodes can only decide once per round
// - Nodes can only finalize once per height
// - Nodes should only propose one value per rounds that they are the nominated proposer for
type executor struct {
	// immutable fields
	namespace    string
	finalizeSeal atomic.Bool
	deciding     atomic.Bool

	mtx     sync.RWMutex
	params  *Parameters
	group   *Group
	height  uint64
	round   uint32
	decided bool

	cancelTimeoutCh chan struct{}
}

func newExecutor(namespace string, height uint64, round uint32, params *Parameters, group *Group) *executor {
	return &executor{
		namespace:       namespace,
		height:          height,
		round:           round,
		params:          params,
		group:           group,
		decided:         false,
		cancelTimeoutCh: make(chan struct{}),
	}
}

// finalize is a concurrently safe method for executing a committed proposal to the application. In response
// the application returns the new paramters and member set to be used for the following height.
// Errors here are considered fatal and the program should immediately halt
func (e *executor) finalize(
	ctx context.Context,
	proposal *Proposal,
	app Application,
	tally *tally,
	decideFn DecideFn,
) error {
	if !e.tryBeginFinalization(proposal.Height) {
		return nil
	}
	defer e.finishFinalization()

	signatures := tally.GetQuorumCommit(proposal.Round)

	resp, err := app.FinalizeData(ctx, &FinalizeDataRequest{
		Height:     proposal.Height,
		Data:       proposal.Data,
		Signatures: signatures,
		MemberSet:  e.group.Export(),
	})
	if err != nil {
		return err
	}
	// The application has requested to gracefully shutdown
	if resp.Terminate {
		return ErrApplicationShutdown
	}
	e.mtx.Lock()
	defer e.mtx.Unlock()
	e.height++
	e.round = 0
	e.decided = false
	if err := e.group.IncrementHeight(proposal.Round, resp.MemberUpdates); err != nil {
		return err
	}
	if resp.Params != nil {
		if err := resp.Params.Validate(); err != nil {
			return err
		}
		e.params = resp.Params
	}
	tally.Reset()
	go e.scheduleTimeout(ctx, e.height, e.round, decideFn)
	return nil
}

func (e *executor) tryBeginFinalization(height uint64) bool {
	e.mtx.RLock()
	defer e.mtx.RUnlock()
	if height != e.height {
		return false
	}
	if !e.finalizeSeal.CompareAndSwap(false, true) {
		return false
	}
	return true
}

func (e *executor) finishFinalization() {
	e.finalizeSeal.Store(false)
}

func (e *executor) isBeingFinalized() bool {
	return e.finalizeSeal.Load()
}

// tryDecide provides a safe serialization point when the consensus engine wants to make a decision
// for a given round. By wrapping the decide logic in this method, it ensures that the consensus engine
// only ever makes a decision once per round. Calls to decide that are concurrent but come immediately after
// the first call are immediately dropped so as to be non blocking.
func (e *executor) tryDecide(ctx context.Context, height uint64, round uint32, decideFn DecideFn) error {
	if !e.tryBeginDecision(height, round) {
		return nil
	}
	defer e.endDecision()

	if err := decideFn(ctx, height, round); err != nil {
		// An error in deciding is considered a failure to decide.
		// errors of these kind are rare and generally indicate something
		// criticial which will most likely lead to the stopping of the node.
		return err
	}

	// It is plausible that we finalize or progress and decide at the same time
	// This would make the update redundant hence we pass through the height and
	// round to double check
	e.markDecided(height, round)
	return nil
}

func (e *executor) tryBeginDecision(height uint64, round uint32) bool {
	e.mtx.RLock()
	defer e.mtx.RUnlock()
	if height != e.height || round != e.round {
		return false
	}
	if e.decided {
		return false
	}
	if !e.deciding.CompareAndSwap(false, true) {
		return false
	}
	return true
}

func (e *executor) endDecision() {
	e.deciding.Store(false)
}

// nextRound is called whenever the node receives a quorum of any votes for that
// particular round.
func (e *executor) nextRound(ctx context.Context, round uint32, decideFn DecideFn) bool {
	e.mtx.Lock()
	defer e.mtx.Unlock()
	if round != e.round+1 {
		return false
	}
	e.round = round
	go e.scheduleTimeout(ctx, e.height, round, decideFn)
	return true
}

// State returns a copy of state
func (e *executor) state() *State {
	e.mtx.RLock()
	defer e.mtx.RUnlock()
	return &State{
		Height:  e.height,
		Round:   e.round,
		Decided: e.decided,
	}
}

func (e *executor) hasDecided() bool {
	e.mtx.RLock()
	defer e.mtx.RUnlock()
	return e.decided
}

func (e *executor) markDecided(height uint64, round uint32) {
	e.mtx.Lock()
	defer e.mtx.Unlock()
	if e.height != height || e.round != round {
		return
	}
	e.decided = true
}

func (e *executor) scheduleTimeout(ctx context.Context, height uint64, round uint32, decideFn DecideFn) {
	timer := time.NewTimer(e.timeout(round))
	select {
	case <-ctx.Done():
		timer.Stop()

	case <-e.cancelTimeoutCh:
		timer.Stop()

	case <-timer.C:
		e.tryDecide(ctx, height, round, decideFn)
	}
}

func (e *executor) cancelAllTimeouts() {
	close(e.cancelTimeoutCh)
}

func (e *executor) timeout(round uint32) time.Duration {
	e.mtx.RLock()
	defer e.mtx.RUnlock()
	return e.params.RoundTimeout.AsDuration() + (e.params.RoundTimeoutDelta.AsDuration() * time.Duration(round))
}

func (e *executor) isProposer(pubKey []byte) bool {
	e.mtx.RLock()
	defer e.mtx.RUnlock()
	proposer := e.group.GetProposer(e.round)
	return bytes.Equal(proposer.PublicKey, pubKey)
}
