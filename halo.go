package halo

import (
	"github.com/cmwaters/halo/consensus"
	"github.com/cmwaters/halo/pkg/app"
	"github.com/cmwaters/halo/pkg/signer"

	"github.com/libp2p/go-libp2p/core/host"
)

func New(app app.Application, host host.Host, signer signer.Signer) *consensus.Engine {
	return &consensus.Engine{}
}
