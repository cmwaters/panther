package tx

import "context"

type Key [32]byte

// Transmitter is a generic protocol which is responsible for ensuring all participants
// in a network eventually receive all transactions.
type Transmitter interface {
	Broadcaster
	Receive(context.Context) ([]byte, error)
	Prune(context.Context, []Key) error
	Invalid(context.Context, Key) error
}

type Broadcaster interface {
	Broadcast(context.Context, []byte) error
}

type Store interface {
	
}