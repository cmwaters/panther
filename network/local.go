package network

import (
	"context"

	"github.com/cmwaters/halo/consensus"
)

type LocalNetwork struct {
}

func NewLocalNetwork() *LocalNetwork {
	return &LocalNetwork{}
}

func (n *LocalNetwork) Gossip(namespace []byte) (Gossip, error) {
	// TODO implement me
	panic("implement me")
}

type LocalGossip struct {
}

func (l *LocalGossip) Close() error {
	// TODO implement me
	panic("implement me")
}

func (l *LocalGossip) BroadcastProposal(ctx context.Context, proposal *consensus.Proposal) error {
	// TODO implement me
	panic("implement me")
}

func (l *LocalGossip) BroadcastVote(ctx context.Context, vote *consensus.Vote) error {
	// TODO implement me
	panic("implement me")
}

func (l *LocalGossip) Notify(notifiee Notifiee) {
	// TODO implement me
	panic("implement me")
}
