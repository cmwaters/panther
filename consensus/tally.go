package consensus

type tally struct {
	// we index payloads by round as that is what votes reference
	proposalByRounds map[uint32]*Proposal
	proposalIDs      map[uint32][]byte
	round            map[uint32]roundTally
	// votes for a proposal we have not yet received
	pendingVotes map[uint32]*Vote
}

func (t *tally) HasVote(vote *Vote) bool {
	return false
}

func (t *tally) AddVote(vote *Vote) (bool, bool) {
	return false, false
}

func (t *tally) AddPendingVote(vote *Vote) {

}

func (t *tally) HasProposal(round uint32) bool {
	return false
}

func (t *tally) GetProposal(round uint32) ([]byte, *Proposal) {
	return nil, nil
}

func (t *tally) AddProposal(proposal *Proposal) {

}

func (t *tally) HasOnlyOneProposal() bool {
	return false
}

func (t *tally) HasQuorumCommit(round uint32) bool {
	return false
}

func (t *tally) GetQuorumCommit(round uint32) [][]byte {
	return nil
}

func (t *tally) HasQuorumSignal(round uint32) bool {
	return false
}

func (t *tally) HasQuorumAny(round uint32) bool {
	return false
}

func (t *tally) Reset() {

}

type roundTally struct {
	// immutable fields
	proposer         []byte
	proposalReceived bool
	hasQuorumAny     bool
	votes            []compactVote
}

type compactVote struct {
	payloadRound uint32
	signature    []byte
}

func (t *roundTally) AddVote(v *Vote) {

}
