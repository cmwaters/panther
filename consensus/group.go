package consensus

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
)

// Group is a collection of members that manages membership and follows
// the weighted round robin leader election algorithm.
type Group struct {
	members          []*Member
	roundWeightings  []roundWeighting
	totalVotingPower int64
}

type roundWeighting struct {
	memberProposerPriorities []int64
	proposerIndex            int
}

// NewGroup creates a group based from a MemberSet
func NewGroup(memberSet *MemberSet) (*Group, error) {
	if len(memberSet.Members) == 0 {
		return nil, errors.New("memberset must have at least one member")
	}

	var totalVotingPower int64 = 0
	firstRoundWeighting := roundWeighting{
		memberProposerPriorities: make([]int64, len(memberSet.Members)),
	}
	var highestProposerPriority int64 = 0
	for idx, m := range memberSet.Members {
		if m.VotingPower == 0 {
			return nil, fmt.Errorf("member %d has 0 voting power", idx)
		}
		totalVotingPower += int64(m.VotingPower)
		firstRoundWeighting.memberProposerPriorities[idx] = m.ProposerPriority
		if m.ProposerPriority > highestProposerPriority {
			highestProposerPriority = m.ProposerPriority
			firstRoundWeighting.proposerIndex = idx
		}
	}

	g := &Group{
		members:          memberSet.Members,
		roundWeightings:  []roundWeighting{firstRoundWeighting},
		totalVotingPower: totalVotingPower,
	}
	g.sort()
	if err := g.checkMemberUniqueness(); err != nil {
		return nil, err
	}
	return g, nil
}

// GetProposer returns a copy of the member that is the proposer of the
// given round.
func (g *Group) GetProposer(round uint32) *Member {
	// make sure we have calculated sufficient rounds. This is a noop if
	// we have already calculated the proposer priorities for that round
	g.addRound(round)

	// Return a copy of the member struct that is the proposer for that
	// round i.e. the member with the highest proposer priority
	return g.members[g.roundWeightings[int(round)].proposerIndex].Copy()
}

// GetMember returns a copy of the member struct from a given index
func (g *Group) GetMember(index uint32) *Member {
	if index >= uint32(len(g.members)) {
		return nil
	}

	return g.members[index].Copy()
}

// IncrementHeight is called when a proposal is committed and the group progress to
// the next height. A set of updates can be made to the groups membership alongside
// the increment to the proposer priority.
// - A voting power of 0, means the member is removed
// - A public key that matches an existing member updates their voting power
// - A new public key creates a new member with the respective voting power.
//
// The fromRound is what round's proposer priorites should be copied across to the new
// set of members. This is the round of the proposal that was eventually committed.
func (g *Group) IncrementHeight(fromRound uint32, memberUpdates []*MemberUpdate) error {
	// copy across the proposer priorities from the fromRound to the members
	g.updateProposerPrioritiesToRound(fromRound)

	// first we iterate through the deletions. Deletions are currently the only manner
	// that this function can error. We cannot mutate state until we know that it can't
	// possibly fail.
	deletedMembers := make([]int, 0, len(g.members))
	for _, m := range memberUpdates {
		// Check if the update is to remove a member
		if m.VotingPower == 0 {
			member, index := g.getMemberByPubKey(m.PublicKey)
			// check that the member exists
			if member == nil {
				return fmt.Errorf("tried to remove member (%X) that does not exist", m.PublicKey)
			}
			deletedMembers = append(deletedMembers, index)
			continue
		}
	}

	// update existing member voting powers and add new members
	for _, m := range g.members {
		if m.VotingPower == 0 {
			continue
		}
		member, _ := g.getMemberByPubKey(m.PublicKey)
		if member == nil {
			// here we are adding a new member, so we append it to the end.
			// we will reorder the members by voting power later
			g.members = append(g.members, &Member{
				PublicKey:   m.PublicKey,
				VotingPower: m.VotingPower,
			})
		} else {
			member.VotingPower = m.VotingPower
		}
	}

	// delete members in reverse order so the index isn't affected by prior removals
	sort.Ints(deletedMembers)
	for i := len(deletedMembers) - 1; i >= 0; i-- {
		memberToRemove := deletedMembers[i]
		g.members = append(g.members[:memberToRemove], g.members[memberToRemove:]...)
	}

	if len(memberUpdates) > 0 {
		// we now have our new membership set. Let's now order them correctly
		g.sort()

		// calculate the new total voting power
		g.calculateTotalVotingPower()
	}

	// Incrementing the height is treated the same as if the round were incremented so we
	// need to update the proposer priorities
	for idx, member := range g.members {
		member.ProposerPriority += int64(member.VotingPower)
		if idx == g.roundWeightings[fromRound].proposerIndex {
			member.ProposerPriority -= g.totalVotingPower
		}
	}

	// now lets create the first rounds proposer weightings and work out the proposer
	g.setFirstRoundsWeightings()
	return nil
}

// Export returns the underlying MemberSet
func (g *Group) Export() *MemberSet {
	return &MemberSet{
		Members: g.members,
	}
}

// TotalVotingPower returns the total voting power of the members
func (g *Group) TotalVotingPower() int64 {
	return g.totalVotingPower
}

// ------------------ PRIVATE FUNCTIONS ---------------------

func (g *Group) addRound(round uint32) {
	for i := len(g.roundWeightings); i <= int(round); i++ {
		g.appendRound()
	}
}

func (g *Group) appendRound() {
	nextRoundWeighting := roundWeighting{
		memberProposerPriorities: make([]int64, len(g.members)),
	}
	previousRound := g.roundWeightings[len(g.roundWeightings)-1]
	// there must always be a member with a non zero priority thus it is safe to start at
	// a priority of 0
	var highestPriority int64 = 0
	for idx, member := range g.members {
		// incremenet the proposer priority by the voting power. The sum proposer priority would have increased
		// by a total of the total voting power of the group
		nextRoundWeighting.memberProposerPriorities[idx] = previousRound.memberProposerPriorities[idx] + int64(member.VotingPower)
		// whichever member was the proposer in the last round has their proposer priority
		// subtracted by the total voting power. This offsets the total voting power which
		// was added in the previous set. Meaning the net difference is 0
		if idx == previousRound.proposerIndex {
			nextRoundWeighting.memberProposerPriorities[idx] -= g.totalVotingPower
		}

		if nextRoundWeighting.memberProposerPriorities[idx] > highestPriority {
			highestPriority = nextRoundWeighting.memberProposerPriorities[idx]
			nextRoundWeighting.proposerIndex = idx
		}
	}
	g.roundWeightings = append(g.roundWeightings, nextRoundWeighting)
}

func (g *Group) getMemberByPubKey(pk []byte) (*Member, int) {
	for idx, m := range g.members {
		if bytes.Equal(pk, m.PublicKey) {
			return m, idx
		}
	}
	return nil, 0
}

func (g *Group) updateProposerPrioritiesToRound(round uint32) {
	if round == 0 {
		// nothing to change as the proposer priorities are the same
		return
	}
	g.addRound(round)
	for idx, member := range g.members {
		member.ProposerPriority = g.roundWeightings[round].memberProposerPriorities[idx]
	}
}

func (g *Group) sort() {
	sort.Slice(g.members, func(i, j int) bool {
		if g.members[i].VotingPower == g.members[j].VotingPower {
			return bytes.Compare(g.members[i].PublicKey, g.members[j].PublicKey) < 0
		}
		return g.members[i].VotingPower < g.members[j].VotingPower
	})
}

func (g *Group) setFirstRoundsWeightings() {
	firstRoundWeighting := roundWeighting{
		memberProposerPriorities: make([]int64, len(g.members)),
	}
	var highestProposerPriority int64 = 0
	for idx, m := range g.members {
		firstRoundWeighting.memberProposerPriorities[idx] = m.ProposerPriority
		if m.ProposerPriority > highestProposerPriority {
			highestProposerPriority = m.ProposerPriority
			firstRoundWeighting.proposerIndex = idx
		}
	}
	g.roundWeightings = []roundWeighting{firstRoundWeighting}
}

// checkMemberUniqueness returns an error if there
// is more that one member with the same public key.
func (g *Group) checkMemberUniqueness() error {
	for i, memberA := range g.members {
		for j, memberB := range g.members {
			if i == j {
				continue
			}
			if bytes.Equal(memberA.PublicKey, memberB.PublicKey) {
				return fmt.Errorf("members %d and %d have the same public key", i, j)
			}
		}
	}
	return nil
}

func (g *Group) calculateTotalVotingPower() {
	var totalVotingPower int64 = 0
	for _, m := range g.members {
		totalVotingPower += int64(m.VotingPower)
	}
	g.totalVotingPower = totalVotingPower
}

func (m *Member) Copy() *Member {
	return &Member{
		PublicKey:        m.PublicKey,
		VotingPower:      m.VotingPower,
		ProposerPriority: m.ProposerPriority,
	}
}
