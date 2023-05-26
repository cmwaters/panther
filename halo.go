package halo

import (
	"github.com/cmwaters/halo/consensus"
	"github.com/cmwaters/halo/network"
	"github.com/cmwaters/halo/pkg/sign"

	"github.com/libp2p/go-libp2p/core/host"
)

func New(host host.Host, signer sign.Signer, parameters consensus.Parameters) *consensus.Engine {
	gossipLayer := network.NewLibP2PGossip(host)
	return consensus.New(gossipLayer, signer, parameters)
}
