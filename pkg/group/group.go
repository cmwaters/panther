package group

type Group interface {
	Proposer(uint) Member
	Member(uint) Member
	TotalWeight() uint64
}

type Member interface {
	ID() []byte
	Weight() uint32
	Verify(msg, sig []byte) bool
}
