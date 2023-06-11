package network

import (
	"context"
	"io"

	"github.com/cmwaters/halo/consensus"
)

// TODO(@Wondertan): Move to independent network pkg once dependency cycle on types is resolved

// Namespace
// TODO(@Wondertan): Actually use it
type Namespace string

type Network interface {
	Gossip(namespace []byte) (Gossip, error)
}

// Gossip is an interface which allows the consensus engine to both broadcast
// and receive messages to and from other nodes in the network. It must eventually
// propagate messages to all non-faulty nodes within the network. The algorithm
// for how this is done i.e. simply flooding the network or using some form of
// content addressing protocol is left to the implementer.
type Gossip interface {
	io.Closer
	Broadcaster
	Notifier
}

type Broadcaster interface {
	BroadcastProposal(context.Context, *consensus.Proposal) error
	BroadcastVote(context.Context, *consensus.Vote) error
}

type Notifier interface {
	// Notify registers Notifiee wishing to receive notifications about new messages.
	// Any non-nil error returned from On... handlers rejects the message as invalid.
	Notify(Notifiee)
}

// Notifiee
// TODO(@Wondertan): The more I look into this, the more I want to abstract
// Proposal and Vote into a Message.
type Notifiee interface {
	OnProposal(context.Context, *consensus.Proposal) error
	OnVote(context.Context, *consensus.Vote) error
}