package consensus

type tally struct {
	// we index payloads by round as that is what votes reference
	payloadByRound map[uint32]*Payload
	payloadIDs map[uint32][]byte
	round map[uint32]roundTally
	pendingVotes map[uint32]*Vote // 
}

func (t *tally) HasVote(vote *Vote) bool {
	return false
}

func (t *tally) AddVote(vote *Vote) (bool, bool) {
	return false, false
}

func (t *tally) HasPayload(round uint32) (bool, []byte) {
	return false, nil
}


type roundTally struct {
	// immutable fields
	proposer []byte
	proposalReceived bool
	hasQuorumAny bool
	votes []compactVote
}

type compactVote struct {
	payloadRound uint32
	signature []byte
}

func (t *roundTally) AddVote(v *Vote) {

}