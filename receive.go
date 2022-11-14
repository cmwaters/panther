package consensus

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
)

// Receive is the the entry point for inbound messages broadcasted by other consensus
// engines within the same network. Any networking implementation should call this method
// for any message sent by another peer. Receive is concurrently safe. Only start calling
// `Receive“ after `Start` has completed.
func (e *Engine) Receive(ctx context.Context, req *ReceiveRequest) (*ReceiveResponse, error) {
	if req == nil || req.Msg == nil {
		return nil, errors.New("received empty request")
	}

	if atomic.LoadUint32(&e.status) != Operating {
		return nil, errors.New("consensus engine not in operation")
	}

	switch m := req.Msg.Sum.(type) {
	case *Msg_Vote:
		if m.Vote == nil {
			return nil, errors.New("received nil vote")
		}
		e.handleVote(ctx, *m.Vote)

	case *Msg_Payload:
		if m.Payload == nil {
			return nil, errors.New("received nil payload")
		}
		e.handlePayload(ctx, *m.Payload)

	default:
		return nil, fmt.Errorf("received unsupported msg: %T", m)

	}

	return nil, nil
}

func (e *Engine) scheduleTimeout() {

}

func (e *Engine) handleTimeout() {

}

func (e *Engine) handlePayload(ctx context.Context, payload Payload) {

}

func (e *Engine) handleVote(ctx context.Context, vote Vote) error {
	if err := vote.ValidateBasic(); err != nil {
		return err
	}

	if vote.Height != e.state.height {
		return fmt.Errorf("vote is of a different height. Expected %d, got %d", e.state.height, vote.Height)
	}

	if int(vote.ValidatorIndex) >= len(e.state.validators.Validators) {
		return fmt.Errorf("invalid validator index exceeds total validators (%d)", vote.ValidatorIndex)
	}

	if e.tally.HasVote(&vote) {
		return errors.New("vote already in tally")
	}

	exists, payloadID := e.tally.HasPayload(vote.ReferenceRound)
	if !exists {
		// TODO: add to a pending queue to be processed once we receive the payload
		return nil
	}

	_, val := e.state.validators.GetByIndex(int32(vote.ValidatorIndex))

	msg := CannonicalizedVoteBytes(&vote, payloadID, e.state.namespace)

	if !e.verifyFunc(val.PubKey.Bytes(), msg, vote.Signature) {
		return fmt.Errorf("invalid signature %X from validator %d for payload %X", vote.Signature, vote.ValidatorIndex, payloadID)
	}

	e.tally.AddVote(&vote)

	// TODO: add state transition handling logic

	return nil
}

func (e *Engine) enterPropose(round uint32)

func (e *Engine) enterPrevote(round uint32)

func (e *Engine) enterPrecommit(round uint32)

func (e *Engine) enterCommit(round uint32)

func (v Vote) ValidateBasic() error {
	if v.ReferenceRound > v.Round {
		return fmt.Errorf("vote in round %d is for a payload in a future round (%d)", v.Round, v.ReferenceRound)
	}

	if v.Type != Vote_PREVOTE || v.Type != Vote_PRECOMMIT {
		return errors.New("vote must be of type prevote or precommit")
	}

	if len(v.Signature) == 0 {
		return errors.New("vote does not contain any signature")
	}

	return nil
}
