package group

type Group interface {
	Proposer(round uint) Member
	Member(index uint) Member
	GetMemberByID(id []byte) (Member, uint)
	TotalWeight() uint64
	Size() int
}

type Member interface {
	ID() []byte
	Weight() uint32
	Verify(msg, sig []byte) bool
}
