package consensus

import "github.com/cmwaters/halo/pkg/group"

type Store struct {
	// we index payloads by round as that is what votes reference
	proposalByRounds map[uint32]*Proposal
	proposalIDs      map[uint32][]byte
	round            map[uint32]roundTally
	// votes for a proposal we have not yet received
	pendingVotes map[uint32]*Vote
}

func NewStore() *Store {
	return &Store{
		proposalByRounds: make(map[uint32]*Proposal),
		proposalIDs:      make(map[uint32][]byte),
		round:            make(map[uint32]roundTally),
		pendingVotes:     make(map[uint32]*Vote),
	}
}

func (s *Store) HasVote(vote *Vote) bool {
	return false
}

func (s *Store) AddVote(vote *Vote) bool {
	return false
}

func (s *Store) AddPendingVote(vote *Vote) {

}

func (s *Store) HasProposal(round uint32) bool {
	return false
}

func (s *Store) GetProposal(round uint32) ([]byte, *Proposal) {
	return nil, nil
}

func (s *Store) AddProposal(proposal *Proposal) {

}

func (s *Store) CreateCommitment(proposalRound uint32) (group.Commitment, error) {
	return nil, nil
}

func (s *Store) Reset() {

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
