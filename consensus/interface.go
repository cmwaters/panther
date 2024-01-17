package consensus

import (
	"context"

	"github.com/cmwaters/panther/pkg/app"
	"github.com/cmwaters/panther/pkg/group"
)

type (
	// Service embodies a process that is perpetually running the underlying consensus
	// protocol. Each iteration is known as a height. The service starts by being specified
	// a height and a state machine that it will perform replication on and continues until
	// an error is encountered, or it is stopped.
	Service interface {
		Start(context.Context, uint64, app.StateMachine) error
		Stop() error
		StopAtHeight(uint64) error
		Wait() <-chan struct{}
	}

	// Consensus is an interface that allows the caller to `Commit` a value, either
	// proposed from it's own process or by a participant in the "Group". Underlying
	// a Consensus instance is a networking layer, responsible for broadcasting proposals
	// and votes to one another.
	Consensus interface {
		Commit(
			context.Context,
			uint64,
			group.Group,
			app.Propose,
			app.VerifyProposal,
		) ([]byte, group.Commitment, error)
	}

	// Gossip is an interface which allows the consensus engine to both broadcast
	// and receive messages to and from other nodes in the network. It must eventually
	// propagate messages to all non-faulty nodes within the network. The algorithm
	// for how this is done i.e. simply flooding the network or using some form of
	// content addressing protocol is left to the implementer.
	Gossip interface {
		Receiver
		Sender
		Reporter
	}

	Receiver interface {
		ReceiveProposal(context.Context, uint64) (*Proposal, error)
		ReceiveVote(context.Context, uint64) (*Vote, error)
	}

	Sender interface {
		BroadcastProposal(context.Context, *Proposal) error
		BroadcastVote(context.Context, *Vote) error
	}

	Reporter interface {
		ReportInvalidProposal(context.Context, *Proposal, error) error
		ReportInvalidVote(context.Context, *Vote, error) error
	}
)
