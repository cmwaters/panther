package consensus

import "time"

// Parameters are a set of consensus level parameters. These can be modified per height of consensus
type Parameters struct {
	// ProposalTimeout is the timeout for a proposal to be received in a round before being deemed invalid.
	// If the timeout is reached, the protocol will vote nil for the round if it is not already locked on
	// another value. It is recommended that the network use the same value for this parameter
	ProposalTimeout time.Duration

	// Upon reaching 2f + 1 LOCK votes for any proposal, the protocol will wait for as long as 
	// "lockDelay", for a quorum of a specific value to manifest itself. If so, the protocol will 
	// lock and vote to COMMIT that value.
	// 
	// A lock delay of 0, will mean that unless the first 2f + 1 in votes all LOCK a value, the 
	// process itself, will COMMIT nil for that round
	LockDelay time.Duration
}

