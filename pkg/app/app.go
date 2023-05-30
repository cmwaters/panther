package app

import (
	"context"

	"github.com/cmwaters/halo/pkg/group"
)

type (
	// StateMachine represents the set of components that the consensus engine interacts
	// with for state machine replication
	StateMachine interface {
		Initialize(context.Context, uint64) (group.Group, error)
		Propose(context.Context, uint64) ([]byte, error)
		VerifyProposal(context.Context, uint64, []byte) error
		Finalize(context.Context, uint64, []byte) error
	}

	// Initialize is called at the start of the each height and provides the
	// group or participants involved in consensus
	Initialize func(context.Context, uint64) (group.Group, error)

	// Propose calls for a proposal to be provided to the consensus engine and
	// put forth to the network.
	Propose func(context.Context, uint64) ([]byte, error)

	// VerifyProposal allows the state machine to also determine
	// whether a proposal is valid or not. If an error is returned, the proposal
	// is deemed invalid and the node votes against it.
	VerifyProposal func(context.Context, uint64, []byte) error

	// Finalize receives agreed upon data from the consensus engine. It may apply
	// it to some state machine or simply persist it. Any error here will cause
	// the consensus engine to gracefully shutdown.
	Finalize func(context.Context, uint64, []byte) error
)
