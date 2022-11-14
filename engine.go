package consensus

import (
	"context"
	"sync/atomic"
)

// Used in the handshake for ensuring compatibility. All consensus engines in a network
// should be on the same version
const Version = 1

type (
	// The application supports the relevant business logic that uses the consensus
	// engine for decentralized state machine replication. Specifically an application
	// must be responsible for at least three things:
	//
	// 1) Forming and aggregation of transactions which are bundled together in a payload
	//    and proposed by the proposer of that round. Applications may also perform
	//    transcation dissemination so that all participants are aware of transactions.
	//    This is useful to ensure transactions are committed faster and for content
	//    addressable payloads which only contain the hashes of transactions.
	//
	// 2) Validation of proposed data. Specifically this must conform with the coherence
	//    property - a correct node shold never propose a transaction that another correct
	//    process would deem as invalid.
	//
	// 3) Execution (and optionally the persistence) of tranasctions finalized by the
	//    consensus engine. For state machine replication, this must be a deterministic
	//    process such that all correct processes upon receiving the same transactions will
	//    always progress to the same state. One can use hashes as a method to detect non-
	//    determinsim.
	//
	// Additionally, the application ideally has a subcomponent responsible for syncing to
	// the height that the rest of the network is at. This involves the sending of payloads
	// and their respective signatures for other nodes to verify.
	Application ApplicationServer

	// Signer is a service that securely manages a nodes private key
	// and signs votes and proposals for the consensus engine.
	//
	// The signer should ensure that the node never double signs. This usually means
	// implementing a high-water mark tracking the height, round and type of vote.
	//
	// Make sure the verify function corresponds to the signature scheme used by
	// the signer
	Signer SignerServer

	// Sender is functionality provided by the networking layer
	// that allows the consensus engine to gossip votes, proposals
	// and payloads to all other nodes within the network
	Sender SenderServer

	// Dictates how signatures from voters should be verified. This needs
	// to match with the key protocol of the signer.
	VerifyFunc func(publicKey, message, signature []byte) bool
)

// Engine is the core struct that performs byzantine fault tolerant state
// machine replication using the Tendermint protocol.
//
// In order to function it depends on a networking implementation that completes
// the Sender and Receiver interfaces, a state machine for building, verifying,
// executing and persisting data and an optional signer which is necessary as a writer
// in the network to sign votes and proposals using a secured private key
//
// Engine can either be bundled in the same process (in the case of a state machine,
// networking layer and signer written in golang) or can be in a separate process
// (in which gRPC is used to communicate).
//
// The engine runs only in memory and is thus not responsible for persistence and crash
// recovery. Each time the application starts it uses the handshake with the application
// to set or restore the height and other parameters the engine needs to continue
type Engine struct {
	// The application the consensus engine is communicating with to provide SMR
	app Application

	// signer is only used if the node is a validator or writer in the network
	// as opposed to a reader or full node, in which case this can be nil.
	// The signer is responsible for signing votes and proposals.
	signer Signer
	// we save our public key so we can recognise when we need to propose.
	ourPubkey []byte

	// sender is the other half of receiver. It is essentially
	// a hook that the consensus engine calls when it wants to
	// broadcast a message. The `Broadcast` function is called
	// both when the node wants to send a newly constructed message
	// to the rest of the network and when relaying a valid message
	// to others in the network
	sender Sender

	// Engine implements the receiver interface which allows the
	// networking layer to push messages received from other peers
	UnimplementedReceiverServer

	// 0 - Off
	// 1 - Starting up
	// 2 - Operating
	// 3 - Shutting down
	status uint32

	// state tracks all consensus state include height, round and step
	// as well as vote tallies, payloads, validators and consensus params.
	// After every height it is reset
	state *state

	tally *tally

	// for verifying vote and proposal signatures. Defaults to ed25519.
	verifyFunc VerifyFunc

	// managing the lifecycle of the timeout routine
	timeoutCloseCh chan struct{}
	timeoutDoneCh  chan struct{}
}

// Options is a set of configurable parameters. If left empty, defaults
// will be used
type Options struct {
	Signer     Signer
	VerifyFunc VerifyFunc
}

// New creates a new consensus engine
func New(app Application, sender Sender, opts Options) *Engine {
	e := &Engine{
		app:            app,
		sender:         sender,
		verifyFunc:     DefaultVerifyFunc(),
		timeoutCloseCh: make(chan struct{}),
		timeoutDoneCh:  make(chan struct{}),
	}
	if opts.VerifyFunc != nil {
		e.verifyFunc = opts.VerifyFunc
	}
	if opts.Signer != nil {
		e.signer = opts.Signer
	}
	return e
}

// Operational phases
const (
	Off = iota
	StartingUp
	Operating
	ShuttingDown
)

// Start begins the consensus engine. It will initiate a handshake with
// the application and potentially with the signer, then will enter the
// height and round provided by the application and begin the timeout routine
// for handling timeouts.
func (e *Engine) Start(ctx context.Context) error {
	if !atomic.CompareAndSwapUint32(&e.status, Off, StartingUp) {
		// engine is already running so nothing to do
		return nil
	}

	resp, err := e.app.Handshake(ctx, &HandshakeRequest{
		Version: Version,
	})
	if err != nil {
		atomic.StoreUint32(&e.status, Off)
		return err
	}

	if e.signer != nil {
		signerResp, err := e.signer.Handshake(ctx, &SignerHandshakeRequest{
			Height:  resp.Height,
			Round:   resp.Round,
			Version: Version,
			Namespace: resp.Namespace,
		})
		if err != nil {
			atomic.StoreUint32(&e.status, Off)
			return err
		}
		e.ourPubkey = signerResp.PubKey
	}

	e.state = newState(resp.Height, resp.Round)

	atomic.SwapUint32(&e.status, Operating)
	return nil
}

// Stop halts the timer and closes all active resources. It is safe to restart
// the consensus engine with `Start`
func (e *Engine) Stop() {
	if !atomic.CompareAndSwapUint32(&e.status, Operating, ShuttingDown) {
		// engine was not running
		return
	}

	close(e.timeoutCloseCh)

	<-e.timeoutDoneCh

	atomic.SwapUint32(&e.status, Off)
}
