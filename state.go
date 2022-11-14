package consensus

import (
	"fmt"
	sync "sync"
	"time"

	"github.com/tendermint/tendermint/types"
)

const (
	ProposeStep uint8 = iota
	PrevoteStep
	PrecommitStep
	CommitStep
)

type state struct {
	// immutable fields
	namespace   string
	height     uint64
	params     *SynchronyParams
	validators *types.ValidatorSet

	mtx   sync.RWMutex
	round uint32
	step  uint8
}

func newState(height uint64, round uint32) *state {
	return &state{
		height: height,
		round:  round,
		step:   ProposeStep,
	}
}

func (s *state) increment() {

}

func (s *state) scheduleTimeout() {
	s.mtx.RLock()
	defer s.mtx.RUnlock()
	height, round, step := s.height, s.round, s.step
	time.AfterFunc(calcTimeout(round, step, s.params), s.timeoutFunc(height, round, step))
}

func (s *state) timeoutFunc(height uint64, round uint32, step uint8) func() {
	return func() {
		s.mtx.RLock()
		if s.height != height || s.round != round || s.step != step {
			s.mtx.RUnlock()
			return
		}
		s.mtx.RUnlock()
		s.increment()
	}
}

func calcTimeout(round uint32, step uint8, params *SynchronyParams) time.Duration {
	switch step {
	case ProposeStep:
		return params.Propose.AsDuration() + (time.Duration(round) * params.ProposeDelta.AsDuration())
	case PrevoteStep, PrecommitStep:
		return params.Vote.AsDuration() + (time.Duration(round) * params.Vote.AsDuration())
	case CommitStep:
		return params.Commit.AsDuration()
	default:
		panic(fmt.Sprintf("unexpected step %d", step))
	}
}
