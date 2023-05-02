package consensus

import "context"

type (
	Service interface {
		Start(ctx context.Context, height uint64) error
		Stop() error
		StopAtHeight(height uint64) error
		Wait() <-chan struct{}
	}

	// Gossip is an interface which allows the consensus engine to both broadcast
	// and receive messages to and from other nodes in the network. It must eventually
	// propagate messages to all non-faulty nodes within the network. The algorithm
	// for how this is done i.e. simply flooding the network or using some form of
	// content addressing protocol is left to the implementer.
	Gossip interface {
		Receiver
		Sender
	}

	Receiver interface {
		ReceiveProposal(context.Context) (*Proposal, error)
		ReceiveVote(context.Context) (*Vote, error)
	}

	Sender interface {
		BroadcastProposal(context.Context, *Proposal) error
		BroadcastVote(context.Context, *Vote) error
	}
)
