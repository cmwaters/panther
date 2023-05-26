package network

import (
	"context"

	"github.com/cmwaters/halo/consensus"
	"github.com/libp2p/go-libp2p/core/host"
)

type LibP2PGossip struct {
	host host.Host
}

func NewLibP2PGossip(host host.Host) consensus.Gossip {
	return &LibP2PGossip{host: host}
}

func (g *LibP2PGossip) ReceiveProposal(context.Context, uint64) (*consensus.Proposal, error)
func (g *LibP2PGossip) ReceiveVote(context.Context, uint64) (*consensus.Vote, error)
func (g *LibP2PGossip) BroadcastProposal(context.Context, *consensus.Proposal) error
func (g *LibP2PGossip) BroadcastVote(context.Context, *consensus.Vote) error
func (g *LibP2PGossip) ReportProposal(context.Context, *consensus.Proposal) error
func (g *LibP2PGossip) ReportVote(context.Context, *consensus.Vote) error
