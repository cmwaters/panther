package consensus

import (
	"context"

	"github.com/cmwaters/halo/pkg/app"
	"github.com/cmwaters/halo/pkg/group"
)

type (
	// REVIEW: The Service iface does not belong in the consensus package and I think you know this.
	// The service iface + BaseService in Tendermint is an interesting pattern, although to my taste a bit
	// over-engineered and I think long term we should define similar Service.
	//
	// I.e. in the node we didn't rushed with Service iface, because we followed new approach for all us,
	// which so far shows to be very nice. Our Service is just Start/Stop(ctx) error and that covers all
	// possible needs. Besides Starting and Stopping this lifecycle methods may do some additional IO
	// inside required to initialize the component. Like, make network request or read something from disk
	// to recover state or similar. The context allows to terminate blocking if for some reason IO takes
	// too long. But that's all you usually need. It also enables cool things like
	// https://github.com/celestiaorg/celestia-node/blob/main/nodebuilder/fraud/lifecycle.go#L21
	//
	// They internally block, so it removes the need for Wait. IsRunning is imo also useless. If component
	// A has runtime access to component B it has to be already started, so that in runtime you don't even
	// need to care if the component you talk is started or not why IsRunning. The StopAtHeight is pretty
	// interesting and seems to a niche usecase. I would just make it so when you Stop then Engine, it
	// just blocks until any running Commits are done and then gracefully stops, but if we need to say
	// teardown at particular height, we could extend introduce HeightService or something that extends
	// the Service with new method, and every component that cares about the height would stop there or
	// we could introduce context option for the Stop.
	//
	// N.B. It's a bit early to discuss this, but sharing my thoughts on this as I had multiple discussions
	// on how to approach this.

	// Service embodies a process that is perpetually running the underlying consensus
	// protocol. Each iteration is known as a height. The service starts by being specified
	// a height and a state machine that it will perform replication on and continues until 
	// an error is encountered, or it is stopped.
	Service interface {
		Start(context.Context, uint64, app.StateMachine) error
		Stop() error
		StopAtHeight(uint64) error
		Wait() <-chan struct{}
		IsRunning() bool
	}

	// REVIEW:
	//  :heart:
	//
	// Just two points:
	//
	//  * consensus` depends on the `app`. Conceptually, `app` depends on
	//  `consensus` and asks it to find it, and it's super intuitive when code follows the
	//  conceptual dependency graph.
	//
	//  * We only return committed data here, but it would be nice
	//  to give the application here the actual Commit in return which contains everyones vote
	//  to cover vote extension. (I know you think Vote Extension is very niche, but we have to support
	//  it be compelling) So I would also carefully define consensus.Commit (or consensus.Commitment)
	//  Does not have to be in the first iteration, but it also feels like refactoring it later will be
	//  painful. Additionally, this would avoid the need for Engine to Store votes, leaving this responsibility
	//  to the higher level caller. I will always advocate for consensus to be as barebone as possible :sunglasses:
	//

	// Consensus is an interface that allows the caller to `Commit` a value, either
	// proposed from it's own process or by a participant in the "Group". Underlying
	// a Consensus instance is a networking layer, responsible for broadcasting proposals
	// and votes to one another.
	Consensus interface {
		Commit(
			context.Context,
			uint64,
			group.Group,
			app.Propose,
			app.VerifyProposal,
		) ([]byte, error)
	}

	// REVIEW:
	//  I think I got the approach here. Every Commit round we create Store, Tally, Verifier,
	//  then spawn the goroutines to await every message type for a particular height.
	//
	//  The Gossip would then need to keep message types in some map and have internal pubsubing
	//  semantics(height -> prop/vote), e.g. if no proposal for height wait/subscribe for it,
	//  while the networking side publishes it once received and publish may as well happen earlier
	//  than subscription. The clean-up model here is either by reporting an invalid message or by
	//  periodic GC that cleans up old messages(I don't see success case reporting in the model).
	//
	//  A few points:
	//
	//  (1) With such model every Gossip implementation would need to implement the same mapping mechanism
	//  with pubsubing(height -> prop/vote). This includes local, GossipSub or some other protocol.
	//  That is, they would have to manage the same mechanism to be able to map unordered
	//  gossiped messages to per height requests.
	//
	//  I propose to move this mapping(and complexity) from Gossip implementors and keep it in some
	//  subcomponent of Engine. The Gossip interface would then be responsible for receiving and
	//  broadcasting messages without being aware of heights.
	//
	//  (2) Multiplexing. Current Gossip interface and design does not enable multiplexing.
	//  We have Engine namespaces, but the Gossip does not know about them, limiting us to only one
	//  consensus instance running at the same time.
	//
	//  I propose to introduce the Network interface enabling multiplexing which would spawn a Gossip
	//  instance per namespace. Instantiation of Gossip may also involve discovering more peers on
	//  participating/listening in the consensus network identified by namespace. The namespace
	//  would also represent Gossip topic in the GossipSub implementation.
	//
	// Applying those two proposals the end types would look like:
	//
	// type Namespace string
	//
	// type Network interface {
	//	Gossip(context.Context, Namespace) (Gossip, error)
	// }
	//
	// type Gossip interface {
	//  Sender // uses the same Sender interface
	//  Notifier // combines functionality of both Receiver and Reporter
	// }
	//
	// type Notifier interface {
	//  // Notify registers Notifiee wishing to receive notifications about new messages.
	//  // Any non-nil error returned from On... handlers rejects the message as invalid.
	//  Notify(Notifiee)
	// }
	//
	// type Notifiee interface {
	//	OnProposal(context.Context, *Proposal) error
	//	OnVote(context.Context, *Vote) error
	// }
	//
	// (3) I feel like there is a value in unifying Vote and Proposal into a more general Message type
	//  within Gossip interfaces, but I don't have any good argument for that, so I don't feel
	//  strongly about it.
	//
	//  The way I see it is what part is responsible for parsing general message to Vote or Proposal.
	//  In case we want to keep the Gossip interfaces single purpose for the consensus only, then
	//  it makes sense to make Network understand specific message types and how and when to deserialize
	//  them. In case we want to keep the network interface more generic for other parts of the system
	//  to reuse than making generic BroadcastMessage instead of BroadcastProposal and BroadcastVote
	//  makes more sense.
	//

	// Gossip is an interface which allows the consensus engine to both broadcast
	// and receive messages to and from other nodes in the network. It must eventually
	// propagate messages to all non-faulty nodes within the network. The algorithm
	// for how this is done i.e. simply flooding the network or using some form of
	// content addressing protocol is left to the implementer.

	Gossip interface {
		Receiver
		Sender
		Reporter
	}

	Receiver interface {
		ReceiveProposal(context.Context, uint64) (*Proposal, error)
		ReceiveVote(context.Context, uint64) (*Vote, error)
	}

	Sender interface {
		BroadcastProposal(context.Context, *Proposal) error
		BroadcastVote(context.Context, *Vote) error
	}

	Reporter interface {
		ReportProposal(context.Context, *Proposal) error
		ReportVote(context.Context, *Vote) error
	}
)
