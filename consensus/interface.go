package consensus

type (
	// The application supports the relevant business logic that uses the consensus
	// engine for decentralized state machine replication. Specifically an application
	// must be responsible for at least three things:
	//
	// 1) Forming and aggregation of transactions which are bundled together in a payload
	//    and proposed by the proposer of that round. Applications may also perform
	//    transcation dissemination so that all participants are aware of transactions.
	//    This is useful to ensure transactions are committed faster and for content
	//    addressable payloads which only contain the hashes of transactions.
	//
	// 2) Validation of proposed data. Specifically this must conform with the coherence
	//    property - a correct node shold never propose a transaction that another correct
	//    process would deem as invalid.
	//
	// 3) Execution (and optionally the persistence) of tranasctions finalized by the
	//    consensus engine. For state machine replication, this must be a deterministic
	//    process such that all correct processes upon receiving the same transactions will
	//    always progress to the same state. One can use hashes as a method to detect non-
	//    determinsim.
	//
	// Additionally, the application ideally has a subcomponent responsible for syncing to
	// the height that the rest of the network is at. This involves the sending of payloads
	// and their respective signatures for other nodes to verify.
	Application ApplicationServer

	// Gossip is an interface which allows the consensus engine to both broadcast
	// and receive messages to and from other nodes in the network. It must eventually
	// propagate messages to all non-faulty nodes within the network. The algorithm
	// for how this is done i.e. simply flooding the network or using some form of
	// content addressing protocol is left to the implementer.
	Gossip interface {
		ReceiverServer
		SenderServer
	}

	// The following are a list of optional interfaces

	// Signer is a service that securely manages a nodes private key
	// and signs votes and proposals for the consensus engine.
	//
	// The signer should ensure that the node never double signs. This usually means
	// implementing a high-water mark tracking the height, round and type of vote.
	//
	// Make sure the verify function corresponds to the signature scheme used by
	// the signer
	Signer SignerServer

	// Dictates how signatures from voters should be verified. This needs
	// to match with the key protocol of the signer.
	VerifyFunc func(publicKey, message, signature []byte) bool
)
