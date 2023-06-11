package p2p

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/cmwaters/halo/consensus"
	"github.com/cmwaters/halo/network"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testNamespace = []byte("ZGODA")

// TestP2PNetwork
// TODO(@Wondertan): This test works solely over API without any knowledge of
// underlying implementation. Make it part of a test suite so that other Network
// implementations can use for their testing.
func TestP2PNetwork(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	t.Cleanup(cancel)

	nets := setupP2PNetworks(ctx, t, 2)
	n0, n1 := nets[0], nets[1]

	g0, err := n0.Gossip(testNamespace)
	require.NoError(t, err)
	g1, err := n1.Gossip(testNamespace)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, g0.Close())
		require.NoError(t, g1.Close())
	})

	nt0, nt1 := makeNotifiee(), makeNotifiee()
	g0.Notify(nt0)
	g1.Notify(nt1)

	// TODO(@Wondertan): Cleanup and deduplicate test logic
	propIn0 := RandProposal()
	err = g0.BroadcastProposal(ctx, propIn0)
	require.NoError(t, err)

	// test valid proposals
	propOut0, err := nt0.RcvProposal(ctx) // ensures we receive msg from ourselves
	require.NoError(t, err)
	require.NotNil(t, propOut0)
	assert.EqualValues(t, propIn0, propOut0)
	propOut0, err = nt1.RcvProposal(ctx)
	require.NoError(t, err)
	require.NotNil(t, propOut0)
	assert.EqualValues(t, propIn0, propOut0)

	propIn1 := RandProposal()
	err = g1.BroadcastProposal(ctx, propIn1)
	require.NoError(t, err)

	propOut1, err := nt1.RcvProposal(ctx) // ensures we receive msg from ourselves
	require.NoError(t, err)
	require.NotNil(t, propOut1)
	assert.EqualValues(t, propIn1, propOut1)
	propOut1, err = nt0.RcvProposal(ctx)
	require.NoError(t, err)
	assert.NotNil(t, propOut1)
	assert.EqualValues(t, propIn1, propOut1)

	// test valid votes
	voteIn0 := RandVote()
	err = g0.BroadcastVote(ctx, voteIn0)
	require.NoError(t, err)

	voteOut0, err := nt0.RcvVote(ctx) // ensures we receive msg from ourselves
	require.NoError(t, err)
	require.NotNil(t, voteOut0)
	assert.EqualValues(t, voteIn0, voteOut0)
	voteOut0, err = nt1.RcvVote(ctx)
	require.NoError(t, err)
	require.NotNil(t, voteOut0)
	assert.EqualValues(t, voteIn0, voteOut0)

	voteIn1 := RandVote()
	err = g1.BroadcastVote(ctx, voteIn1)
	require.NoError(t, err)

	voteOut1, err := nt0.RcvVote(ctx) // ensures we receive msg from ourselves
	require.NoError(t, err)
	require.NotNil(t, voteOut1)
	assert.EqualValues(t, voteIn1, voteOut1)
	voteOut1, err = nt1.RcvVote(ctx)
	require.NoError(t, err)
	require.NotNil(t, voteOut1)
	assert.EqualValues(t, voteIn1, voteOut1)

	// test invalid proposal
	invalidProp := RandProposal()
	nt0.validateProp = func(prop *consensus.Proposal) error { // faking validness
		if prop.Height == invalidProp.Height {
			return fmt.Errorf("invalid height")
		}
		return nil
	}
	err = g0.BroadcastProposal(ctx, invalidProp)
	assert.Error(t, err)

	// test invalid vote
	invalidVote := RandVote()
	nt0.validateVote = func(vote *consensus.Vote) error { // faking validness
		if vote.Height == invalidVote.Height {
			return fmt.Errorf("invalid height")
		}
		return nil
	}
	err = g0.BroadcastVote(ctx, invalidVote)
	assert.Error(t, err)
}

type notifiee struct {
	props chan *consensus.Proposal
	votes chan *consensus.Vote

	validateProp func(*consensus.Proposal) error
	validateVote func(*consensus.Vote) error
}

func makeNotifiee() *notifiee {
	return &notifiee{
		props: make(chan *consensus.Proposal, 1),
		votes: make(chan *consensus.Vote, 1),
		validateProp: func(proposal *consensus.Proposal) error {
			return nil
		},
		validateVote: func(proposal *consensus.Vote) error {
			return nil
		},
	}
}

func (n *notifiee) RcvProposal(ctx context.Context) (*consensus.Proposal, error) {
	select {
	case prop := <-n.props:
		return prop, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (n *notifiee) RcvVote(ctx context.Context) (*consensus.Vote, error) {
	select {
	case vote := <-n.votes:
		return vote, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (n *notifiee) OnProposal(ctx context.Context, prop *consensus.Proposal) error {
	if err := n.validateProp(prop); err != nil {
		return err
	}
	select {
	case n.props <- prop:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (n *notifiee) OnVote(ctx context.Context, vote *consensus.Vote) error {
	if err := n.validateVote(vote); err != nil {
		return err
	}
	select {
	case n.votes <- vote:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// TODO(@Wondertan): Move these helpers out

func RandVote() *consensus.Vote {
	return &consensus.Vote{
		Height:        rand.Uint64(),
		Round:         rand.Uint32(),
		ProposalRound: rand.Uint32(),
		MemberIndex:   rand.Uint32(),
		Signature:     RandBytes(32),
		Commit:        rand.Intn(2) == 0,
	}
}

func RandProposal() *consensus.Proposal {
	return &consensus.Proposal{
		Data:      RandBytes(50),
		Height:    rand.Uint64(),
		Round:     rand.Uint32(),
		Signature: RandBytes(32),
	}
}

func RandBytes(n int) []byte {
	bs := make([]byte, n)
	for i := 0; i < len(bs); i++ {
		bs[i] = byte(rand.Int() & 0xFF)
	}
	return bs
}

func setupP2PNetworks(ctx context.Context, t *testing.T, n int) []network.Network {
	mn, err := mocknet.FullMeshLinked(n)
	require.NoError(t, err)

	nets := make([]network.Network, n)
	for i := range nets {
		ps, err := pubsub.NewGossipSub(ctx, mn.Hosts()[i])
		require.NoError(t, err)
		nets[i] = NewNetwork(ps)
	}

	err = mn.ConnectAllButSelf()
	require.NoError(t, err)
	return nets
}
