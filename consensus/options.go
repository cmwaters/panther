package consensus

import "crypto"

// REVIEW: I propose to use the multihash from the start.
// https://multiformats.io/multihash/
// We can discuss the trade-ofs in details.

// Option is a set of configurable parameters. If left empty, defaults
// will be used
type Option func(e *Engine)

// WithHashFunc sets the hash function for hashing proposal data
func WithHashFunc(f crypto.Hash) Option {
	return func(e *Engine) {
		e.hasher = f
	}
}

const DefaultHashFunc = crypto.SHA256
