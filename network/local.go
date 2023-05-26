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

func (n *LocalNetwork) New() *LocalGossip {
	return &LocalGossip{}
}

type LocalGossip struct {
}

func (g *LocalGossip) ReceiveProposal(context.Context, uint64) (*consensus.Proposal, error)
func (g *LocalGossip) ReceiveVote(context.Context, uint64) (*consensus.Vote, error)
func (g *LocalGossip) BroadcastProposal(context.Context, *consensus.Proposal) error
func (g *LocalGossip) BroadcastVote(context.Context, *consensus.Vote) error
func (g *LocalGossip) ReportProposal(context.Context, *consensus.Proposal) error
func (g *LocalGossip) ReportVote(context.Context, *consensus.Vote) error
