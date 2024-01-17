package consensus

import (
	"context"
	"errors"
	"fmt"

	"github.com/cmwaters/halo/pkg/group"
	"github.com/cmwaters/halo/pkg/sign"
)

// voteFn returns a function that the tally will call whenever a state transition results in
// issuing a vote. Concretely, this will be called upon receiving a proposal, a timeout or a
// set of votes for a proposal
func (e *Engine) voteFn(
	height uint64,
	group group.Group,
	store *Store,
) (VoteFn, uint32) {
	if e.signer == nil {
		// cannot vote without a signer
		return nil, 0
	}

	id := e.signer.ID()
	member, memberIdx := group.GetMemberByID(id)
	// cannot vote if the node is not a member within the group
	if member == nil {
		return nil, 0
	}

	return func(ctx context.Context, round uint32, proposalRound uint32, commit bool) error {
		vote := NewVote(height, round, proposalRound, uint32(memberIdx), commit)

		// a vote signs over the hash of the bytes that the data proposes. This way a process can only
		// vote (and verify a vote) if they have received the proposal
		dataDigest := store.GetProposalID(proposalRound)
		if dataDigest == nil {
			return fmt.Errorf("proposal %d/%d for vote not found", height, proposalRound)
		}

		signBytes := vote.SignBytes(dataDigest, e.namespace)
		signature, err := e.signer.Sign(ctx, vote.Level(), signBytes)
		if err != nil {
			return fmt.Errorf("signing vote: %w", err)
		}
		vote.Signature = signature

		// sanity check that this function constructs a correctly formed vote
		if err := vote.ValidateForm(); err != nil {
			panic(err)
		}

		if err := e.gossip.BroadcastVote(ctx, vote); err != nil {
			return fmt.Errorf("broadcasting vote: %w", err)
		}

		// add the vote to the store
		store.AddVote(vote)

		return nil
	}, member.Weight()
}

type Vote struct {
	Height        uint64
	Round         uint32
	ProposalRound uint32
	MemberIndex   uint32
	Signature     []byte
	Commit        bool
}

func NewVote(height uint64, round, proposalRound, memberIndex uint32, commit bool) *Vote {
	return &Vote{
		Height:        height,
		Round:         round,
		ProposalRound: proposalRound,
		MemberIndex:   memberIndex,
		Commit:        commit,
	}
}

func (v Vote) SignBytes(digest, namespace []byte) []byte {
	voteType := LOCK_TYPE
	if v.Commit {
		voteType = COMMIT_TYPE
	}
	return EncodeMsgToSign(uint8(voteType), v.Height, v.Round, digest, namespace)
}

func (v Vote) Level() sign.Watermark {
	step := 1
	if v.Commit {
		step = 2
	}
	return sign.Watermark{v.Height, uint64(v.Round), uint64(step)}
}

func (v Vote) ValidateForm() error {
	if v.ProposalRound > v.Round {
		return fmt.Errorf("vote in round %d is for a proposal in a future round (%d)", v.Round, v.ProposalRound)
	}

	if v.Height == 0 {
		return errors.New("vote height is zero")
	}

	if v.Round == 0 {
		return errors.New("vote round is zero")
	}

	if v.ProposalRound == 0 {
		return errors.New("proposal round is zero")
	}

	if len(v.Signature) == 0 {
		return errors.New("vote does not contain any signature")
	}

	return nil
}

func (v Vote) IsCommit() bool {
	return v.Commit
}

func (v *Vote) String() string {
	if v == nil {
		return "nil"
	}

	if v.Commit {
		return fmt.Sprintf("Commit{%d/%d/%d by %d $ %X}", v.Height, v.Round, v.ProposalRound, v.MemberIndex, v.Signature)
	}
	return fmt.Sprintf("Lock{%d/%d/%d by %d $ %X}", v.Height, v.Round, v.ProposalRound, v.MemberIndex, v.Signature)
}
