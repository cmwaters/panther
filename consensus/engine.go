package consensus

import (
	"context"
	"crypto"
	"errors"
	"fmt"
	"os"
	"sync/atomic"

	"github.com/cmwaters/halo/pkg/app"
	"github.com/cmwaters/halo/pkg/group"
	"github.com/cmwaters/halo/pkg/sign"
	"github.com/rs/zerolog"
)

// Used in the handshake for ensuring compatibility. All consensus engines in a network
// should be on the same version
const Version = 1

var _ Service = &Engine{}
var _ Consensus = &Engine{}

// Engine is the core struct that performs byzantine fault tolerant state
// machine replication using the Lock and Commit protocol.
//
// In order to function it depends on a networking implementation that completes
// the Gossip interface, a state machine for building, verifying, executing and
// persisting data and an optional signer which is necessary as a writer
// in the network to sign votes and proposals using a secured private key
//
// The engine runs only in memory and is thus not responsible for persistence and crash
// recovery. Each time start is called, a height and state machine is provided
type Engine struct {
	// namespace represents the unique id of the application. It is paired with
	// height and round to signal uniqueness of proposals and votes
	namespace []byte

	// gossip represents a simple networking abstraction for broadcasting messages
	// that should eventually propagate to all non-faulty nodes in the network as
	// well as eventually receiving all messages generated from other nodes.
	gossip Gossip

	// signer is only used if the node is a validator or writer in the network
	// as opposed to a reader or full node, in which case this can be nil.
	// The signer is responsible for signing votes and proposals.
	signer sign.Signer

	// parameters entails the set of consensus specfic parameters that are used
	// to reach consensus
	parameters Parameters

	// hasher defines how proposal data is hashed for signing
	hasher crypto.Hash

	// status tracks if the engine is running or not.
	status atomic.Bool

	// The following are used for managing the lifecycle of the engine
	cancel context.CancelFunc
	done   chan struct{}

	logger zerolog.Logger
}

// New creates a new consensus engine
func New(gossip Gossip, signer sign.Signer, parameters Parameters, opts ...Option) *Engine {
	e := &Engine{
		gossip:     gossip,
		signer:     signer,
		parameters: parameters,
		hasher:     DefaultHashFunc,
		logger:     zerolog.New(os.Stdout),
	}

	for _, opt := range opts {
		opt(e)
	}

	return e
}

// Operational phases
const (
	Off = false
	On  = true
)

// Start implements the Service interface and starts the consensus engine
func (e *Engine) Start(ctx context.Context, height uint64, app app.StateMachine) error {
	if !e.status.CompareAndSwap(Off, On) {
		return errors.New("engine already running")
	}
	defer e.status.CompareAndSwap(On, Off)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	e.cancel = cancel
	e.done = make(chan struct{})
	defer close(e.done)
	for {
		group, err := app.Initialize(ctx, height)
		if err != nil {
			return err
		}
		data, err := e.Commit(ctx, height, group, app.Propose, app.VerifyProposal)
		if err != nil {
			return err
		}
		if err := app.Finalize(ctx, height, data); err != nil {
			return err
		}
		height++
	}
}

// Commit implements the Consensus interface. It executes a single instance of consensus between
// a group of participants. It takes in two hooks, one for proposing data and another for verifying
// the data. It returns the data that was agreed upon by the group under the lock and commit protocol
func (e *Engine) Commit(
	ctx context.Context,
	height uint64,
	group group.Group,
	proposeFn app.Propose,
	verifyProposalFn app.VerifyProposal,
) ([]byte, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// for each height, the process splits up the task through three components:
	// - Verifier: responsible for verifying the validity of proposals and votes
	// - Store: responsible for storing proposals and votes
	// - Tally: responsible for tallying votes and proposals and for voting and
	//          proposing according to the rules of the consensus protocol
	verifier := NewVerifier(e.namespace, height, group, e.hasher, verifyProposalFn)
	store := NewStore()
	voteFn, selfWeight := e.voteFn(height, group, store)
	tally := NewTally(
		voteFn,
		e.proposeFn(height, group, store, proposeFn),
		selfWeight,
		group.TotalWeight(),
		e.parameters.ProposalTimeout,
		e.parameters.LockDelay,
	)
	
	// The concurrency model of the system is relatively simple. There are three
	// main go routines. The first two: receiveProposals and receiveVotes spawn 
	// new threads for handling each vote and proposal thus handling, verfication 
	// and storage are all highly parallelized. The last, the tally is run in a 
	// separate thread. It uses a single channel to serailize all processed votes 
	// and proposals and follow the logic of the cosensus protocol. When tally.Run() 
	// finalizes a value, the round of the proposal is returned through the `Done`
	// channel.
	errCh := make(chan error, 3)
	go func() {
		errCh <- tally.Run(ctx)
	}()
	go func() {
		errCh <- e.receiveProposals(ctx, tally, verifier, store)
	}()
	go func() {
		errCh <- e.receiveVotes(ctx, tally, verifier, store)
	}()

	// There can be at most three errors for each of the goroutines.
	// If any of them return an error, the `Commit` method errors,
	// the remaining go routines will exit as the context is cancelled.
	for i := 0; i < 3; i++ {
		select {
		case err := <-errCh:
			if err != nil {
				return nil, err
			}
		
		case proposalRound := <-tally.Done():
			_, proposal := store.GetProposal(proposalRound)
			return proposal.Data, nil
		}
	}
	return nil, nil
}

func (e *Engine) Stop() error {
	if !e.status.CompareAndSwap(On, Off) {
		return errors.New("engine is not running")
	}
	e.cancel()
	<-e.Wait()
	return nil
}

func (e *Engine) StopAtHeight(height uint64) error {
	panic("not implemented")
}

func (e *Engine) IsRunning() bool {
	return e.status.Load()
}

func (e *Engine) Wait() <-chan struct{} {
	return e.done
}

var ErrApplicationShutdown = errors.New("application requested termination")

// unrecoverable errors indicate that the consensus engine
// is in a state that is not recoverable. It thus logs the
// error and shutsdown.
type errUnrecoverable struct {
	err error
}

func unrecoverable(err error) error {
	return errUnrecoverable{
		err: err,
	}
}

func isUnrecoverable(err error) bool {
	_, ok := err.(errUnrecoverable)
	return ok
}

func (e errUnrecoverable) Error() string {
	return fmt.Sprintf("unrecoverable error: %w", e.err)
}
