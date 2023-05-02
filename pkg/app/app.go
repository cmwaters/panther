package app

import (
	"context"

	"github.com/cmwaters/halo/pkg/group"
)

type (
	// Application represents the set of components that the consensus engine interacts
	// with for state machine replication
	Application interface {
		Writer
		Reader
		Verifier
		Membership
	}

	// Writer receives agreed upon data from the consensus engine. It may apply
	// it to some state machine or simply persist it. Any error here will cause
	// the consensus engine to gracefully shutdown. All writes are accompanied
	// by a monotonically increasing height.
	Writer interface {
		Write(context.Context, uint64, []byte) error
	}

	// Verifier is an optional interface which can add additional constraints to
	// whether a proposal is valid or not. If an error is returned, the proposal
	// is deemed invalid and the node votes against it.
	Verifier interface {
		Verify(context.Context, uint64, []byte) error
	}

	// Reader acts as the source of the data in the system. If a node is the
	// proposer for this height and round it will call `Read` and sign and broadcast
	// its proposal to peers. An error here will cause the proposer to propose nil.
	Reader interface {
		Read(context.Context, uint64) ([]byte, error)
	}

	// Membership encapsulates the rules that determine the group for a height. The
	// Group is the set of known members that propose and vote according to the
	// consensus protocol. An error here will cause the consensus engine to gracefully
	// shutdown.
	Membership interface {
		Next(context.Context, uint64, group.Group) (group.Group, error)
		Self() []byte
	}
)

type NoopVerifier struct{}

func (v *NoopVerifier) Verify(ctx context.Context, data []byte) error {
	return nil
}
