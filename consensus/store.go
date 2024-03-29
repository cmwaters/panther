package consensus

import (
	"fmt"
	"sync"

	"github.com/cmwaters/panther/pkg/group"
)

type Store struct {
	groupSize int
	// roundLimit is how many rounds ahead of the latest round
	// should the store accept votes for
	roundLimit uint32

	proposalMtx sync.Mutex
	// we index payloads by round as that is what votes reference
	latestRound      uint32
	proposalByRounds map[uint32]*Proposal
	proposalIDs      map[uint32][]byte

	voteMtx sync.Mutex
	votes   map[uint32]map[uint32]compactVote
	// votes for a proposal we have not yet received - proposal_round/round/member_index/vote
	pendingVotes           map[uint32]map[uint32]*Vote
	pendingVotesByProposal map[uint32][]*Vote
}

func NewStore(groupSize int) *Store {
	return &Store{
		groupSize:              groupSize,
		roundLimit:             2,
		latestRound:            0,
		proposalByRounds:       make(map[uint32]*Proposal),
		proposalIDs:            make(map[uint32][]byte),
		votes:                  make(map[uint32]map[uint32]compactVote),
		pendingVotes:           make(map[uint32]map[uint32]*Vote),
		pendingVotesByProposal: make(map[uint32][]*Vote),
	}
}

func (s *Store) HasVote(vote *Vote) bool {
	s.voteMtx.Lock()
	defer s.voteMtx.Unlock()
	votes, ok := s.votes[vote.Round]
	if !ok {
		return false
	}
	_, ok = votes[vote.MemberIndex]
	return !ok
	// TODO: should check for potential equivocation here
}

func (s *Store) AddVote(vote *Vote) {
	s.voteMtx.Lock()
	defer s.voteMtx.Unlock()
	_, ok := s.votes[vote.Round]
	if !ok {
		s.votes[vote.Round] = make(map[uint32]compactVote)
	}
	s.votes[vote.Round][vote.MemberIndex] = compactVote{
		proposalRound: vote.ProposalRound,
		signature:     vote.Signature,
	}
}

func (s *Store) AddPendingVote(vote *Vote) error {
	s.voteMtx.Lock()
	defer s.voteMtx.Unlock()
	if vote.Round > s.latestRound+s.roundLimit {
		return fmt.Errorf("vote from future round %d is too far ahead of latest round %d", vote.Round, s.latestRound)
	}

	_, ok := s.pendingVotes[vote.Round]
	if !ok {
		s.pendingVotes[vote.Round] = make(map[uint32]*Vote)
	}
	s.pendingVotes[vote.Round][vote.MemberIndex] = vote

	s.pendingVotesByProposal[vote.ProposalRound] = append(s.pendingVotesByProposal[vote.ProposalRound], vote)
	return nil
}

func (s *Store) HasProposal(round uint32) bool {
	s.proposalMtx.Lock()
	defer s.proposalMtx.Unlock()
	_, ok := s.proposalIDs[round]
	return ok
}

func (s *Store) GetProposal(round uint32) *Proposal {
	s.proposalMtx.Lock()
	defer s.proposalMtx.Unlock()
	proposal, ok := s.proposalByRounds[round]
	if !ok {
		return nil
	}
	return proposal
}

func (s *Store) GetProposalID(round uint32) []byte {
	s.proposalMtx.Lock()
	defer s.proposalMtx.Unlock()
	id, ok := s.proposalIDs[round]
	if !ok {
		return nil
	}
	return id
}

func (s *Store) AddProposal(proposal *Proposal, id []byte) {
	s.proposalMtx.Lock()
	defer s.proposalMtx.Unlock()
	s.proposalByRounds[proposal.Round] = proposal
	s.proposalIDs[proposal.Round] = id
	if proposal.Round > s.latestRound {
		s.latestRound = proposal.Round
	}
}

func (s *Store) CreateCommitment(proposalRound, commitRound uint32) (group.Commitment, error) {
	signatures := make([][]byte, s.groupSize)
	votes, ok := s.votes[commitRound]
	if !ok {
		return nil, fmt.Errorf("no votes received in round %d", commitRound)
	}

	for i := 0; i < s.groupSize; i++ {
		compactVote, ok := votes[uint32(i)]
		if !ok {
			signatures[i] = nil
		}

		if compactVote.proposalRound != proposalRound {
			signatures[i] = nil
		}

		signatures[i] = compactVote.signature
	}

	return group.NewSignatureSet(signatures), nil
}

type compactVote struct {
	proposalRound uint32
	signature     []byte
}
