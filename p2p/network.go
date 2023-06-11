package p2p

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/cmwaters/halo/consensus"
	"github.com/cmwaters/halo/network"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
)

var _ network.Network = (*Network)(nil)

type Network struct {
	ps *pubsub.PubSub
}

func NewNetwork(ps *pubsub.PubSub) network.Network {
	return &Network{
		ps: ps,
	}
}

func (pn *Network) Gossip(namespace []byte) (network.Gossip, error) {
	// TODO(@Wondertan): network ID prefix, so that we don't collide with random namespaces
	//  that turn out to be the same
	topic, err := pn.ps.Join(string(namespace))
	if err != nil {
		return nil, err
	}

	pg := &Gossip{
		ps: pn.ps,
		tp: topic,
	}
	pg.ensureSubscribed()
	return pg, nil
}

type Gossip struct {
	ps  *pubsub.PubSub
	tp  *pubsub.Topic
	sub *pubsub.Subscription
}

func (p *Gossip) BroadcastProposal(ctx context.Context, proposal *consensus.Proposal) error {
	msg := &message{
		Type:     proposalType,
		Proposal: proposal,
	}
	// FIXME(@Wondertan): This is temporal. Use better serialization protocol
	//  We have a chance to rethink Tendermint's usage of Protobuf !!!
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	// so that we publish when we have at least one peer
	opt := pubsub.WithReadiness(pubsub.MinTopicSize(1))
	return p.tp.Publish(ctx, data, opt)
}

func (p *Gossip) BroadcastVote(ctx context.Context, vote *consensus.Vote) error {
	msg := &message{
		Type: voteType,
		Vote: vote,
	}

	// FIXME(@Wondertan): This is temporal. Use better serialization protocol
	//  We have a chance to rethink Tendermint's usage of Protobuf !!!
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	// so that we publish when we have at least one peer
	opt := pubsub.WithReadiness(pubsub.MinTopicSize(1))
	return p.tp.Publish(ctx, data, opt)
}

func (p *Gossip) Notify(notifiee network.Notifiee) {
	// error can be safely ignored
	_ = p.ps.RegisterTopicValidator(p.tp.String(), func(ctx context.Context, _ peer.ID, pmsg *pubsub.Message) pubsub.ValidationResult {
		var cmsg message
		err := json.Unmarshal(pmsg.Data, &cmsg)
		if err != nil {
			return pubsub.ValidationReject
		}

		switch cmsg.Type {
		case proposalType:
			err = notifiee.OnProposal(ctx, cmsg.Proposal)
			if err != nil {
				return pubsub.ValidationReject
			}
		case voteType:
			err = notifiee.OnVote(ctx, cmsg.Vote)
			if err != nil {
				return pubsub.ValidationReject
			}
		default:
			return pubsub.ValidationReject
		}

		return pubsub.ValidationAccept
	})
}

func (p *Gossip) Close() (err error) {
	p.sub.Cancel()
	err = errors.Join(err, p.ps.UnregisterTopicValidator(p.tp.String()))
	err = errors.Join(err, p.tp.Close())
	return err
}

// ensureSubscribed maintains one and only subscription for the topic
// PubSub requires at least one subscription in order to work correctly.
// The Network interface does not need the notion of subscribers and relies
// only on validators.
func (p *Gossip) ensureSubscribed() {
	sub, err := p.tp.Subscribe()
	if err != nil {
		return // safe to ignore
	}
	p.sub = sub

	go func() {
		for {
			_, err := sub.Next(context.Background())
			if err != nil {
				// happens when subscription is canceled
				return
			}
			// simply ignore messages
		}
	}()
}

type messageType uint8

const (
	proposalType messageType = iota + 1
	voteType
)

type message struct {
	Type     messageType
	Proposal *consensus.Proposal
	Vote     *consensus.Vote
}
