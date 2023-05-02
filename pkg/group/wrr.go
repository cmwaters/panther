package group

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
)

var _ Group = &WeightedRoundRobinGroup{}

// Group is a collection of members that manages membership and follows
// the weighted round robin leader election algorithm.
type WeightedRoundRobinGroup struct {
	members          []Member
	roundWeightings  []roundWeighting
	totalVotingPower uint64
}

type roundWeighting struct {
	memberProposerPriorities []int64
	proposerIndex            int
}

// NewGroup creates a group based from a MemberSet
func NewWeighterRoundRobinGroup(memberSet []Member) (*WeightedRoundRobinGroup, error) {
	if len(memberSet) == 0 {
		return nil, errors.New("memberset must have at least one member")
	}

	var totalVotingPower uint64 = 0
	firstRoundWeighting := roundWeighting{
		memberProposerPriorities: make([]int64, len(memberSet)),
	}
	var highestProposerPriority uint32 = 0
	for idx, m := range memberSet {
		if m.Weight() == 0 {
			return nil, fmt.Errorf("member %d has 0 voting power", idx)
		}
		totalVotingPower += uint64(m.Weight())
		firstRoundWeighting.memberProposerPriorities[idx] = int64(m.Weight())
		if m.Weight() > highestProposerPriority {
			highestProposerPriority = m.Weight()
			firstRoundWeighting.proposerIndex = idx
		}
	}

	g := &WeightedRoundRobinGroup{
		members:          memberSet,
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
func (g *WeightedRoundRobinGroup) Proposer(round uint) Member {
	// make sure we have calculated sufficient rounds. This is a noop if
	// we have already calculated the proposer priorities for that round
	g.addRound(uint32(round))

	// Return a copy of the member struct that is the proposer for that
	// round i.e. the member with the highest proposer priority
	return g.members[g.roundWeightings[int(round)].proposerIndex]
}

// GetMember returns a copy of the member struct from a given index
func (g *WeightedRoundRobinGroup) Member(index uint) Member {
	if index >= uint(len(g.members)) {
		return nil
	}

	return g.members[index]
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
func (g *WeightedRoundRobinGroup) IncrementHeight(fromRound uint32, memberUpdates []Member) error {
	// get the proposer and proposer priorities for the round that the proposal was committed
	pp := g.getProposerPriorities(fromRound)
	lastProposer := g.Proposer(uint(fromRound))

	// first we iterate through the deletions. Deletions are currently the only manner
	// that this function can error. We cannot mutate state until we know that it can't
	// possibly fail.
	deletedMembers := make([]int, 0, len(g.members))
	for _, m := range memberUpdates {
		// Check if the update is to remove a member
		if m.Weight() == 0 {
			member, index := g.getMemberByID(m.ID())
			// check that the member exists
			if member == nil {
				return fmt.Errorf("tried to remove member (%X) that does not exist", m.ID())
			}
			deletedMembers = append(deletedMembers, index)
			continue
		}
	}

	// update existing member voting powers and add new members
	for _, m := range memberUpdates {
		if m.Weight() == 0 {
			continue
		}
		member, _ := g.getMemberByID(m.ID())
		if member == nil {
			// here we are adding a new member, so we append it to the end.
			// we will reorder the members by voting power later
			g.members = append(g.members, m)
		} else {
			member = m
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

	firstRoundWeighting := roundWeighting{
		memberProposerPriorities: make([]int64, len(g.members)),
	}
	var highestProposerPriority int64 = 0
	for i, member := range g.members {
		// we set the proposer priority as members weight plus the proposer priority of the
		// previous height if they were a member then.
		firstRoundWeighting.memberProposerPriorities[i] = int64(member.Weight())
		if priority, ok := pp[string(member.ID())]; ok {
			firstRoundWeighting.memberProposerPriorities[i] += priority
		}
		// the last proposer is offset by the new total voting power
		if bytes.Equal(member.ID(), lastProposer.ID()) {
			firstRoundWeighting.memberProposerPriorities[i] -= int64(g.totalVotingPower)
		}

		// If the member has the highest proposer priority we mark it's index as the proposer
		// for the first round of the new height
		if firstRoundWeighting.memberProposerPriorities[i] > highestProposerPriority {
			highestProposerPriority = firstRoundWeighting.memberProposerPriorities[i]
			firstRoundWeighting.proposerIndex = i
		}
	}
	g.roundWeightings = []roundWeighting{firstRoundWeighting}

	return nil
}

// Export returns the underlying MemberSet
func (g *WeightedRoundRobinGroup) Members() []Member {
	return g.members
}

// TotalVotingPower returns the total voting power of the members
func (g *WeightedRoundRobinGroup) TotalWeight() uint64 {
	return g.totalVotingPower
}

// ------------------ PRIVATE FUNCTIONS ---------------------

func (g *WeightedRoundRobinGroup) addRound(round uint32) {
	for i := len(g.roundWeightings); i <= int(round); i++ {
		g.appendRound()
	}
}

func (g *WeightedRoundRobinGroup) appendRound() {
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
		nextRoundWeighting.memberProposerPriorities[idx] = previousRound.memberProposerPriorities[idx] + int64(member.Weight())
		// whichever member was the proposer in the last round has their proposer priority
		// subtracted by the total voting power. This offsets the total voting power which
		// was added in the previous set. Meaning the net difference is 0
		if idx == previousRound.proposerIndex {
			nextRoundWeighting.memberProposerPriorities[idx] -= int64(g.totalVotingPower)
		}

		if nextRoundWeighting.memberProposerPriorities[idx] > highestPriority {
			highestPriority = nextRoundWeighting.memberProposerPriorities[idx]
			nextRoundWeighting.proposerIndex = idx
		}
	}
	g.roundWeightings = append(g.roundWeightings, nextRoundWeighting)
}

func (g *WeightedRoundRobinGroup) getMemberByID(id []byte) (Member, int) {
	for idx, m := range g.members {
		if bytes.Equal(id, m.ID()) {
			return m, idx
		}
	}
	return nil, 0
}

func (g *WeightedRoundRobinGroup) sort() {
	sort.Slice(g.members, func(i, j int) bool {
		if g.members[i].Weight() == g.members[j].Weight() {
			return bytes.Compare(g.members[i].ID(), g.members[j].ID()) < 0
		}
		return g.members[i].Weight() < g.members[j].Weight()
	})
}

func (g *WeightedRoundRobinGroup) getProposerPriorities(round uint32) map[string]int64 {
	if round >= uint32(len(g.roundWeightings)) {
		g.addRound(round)
	}

	proposerPriorities := make(map[string]int64)
	for idx, priority := range g.roundWeightings[round].memberProposerPriorities {
		proposerPriorities[string(g.members[idx].ID())] = priority
	}
	return proposerPriorities
}

// checkMemberUniqueness returns an error if there
// is more that one member with the same id.
func (g *WeightedRoundRobinGroup) checkMemberUniqueness() error {
	for i, memberA := range g.members {
		for j, memberB := range g.members {
			if i == j {
				continue
			}
			if bytes.Equal(memberA.ID(), memberB.ID()) {
				return fmt.Errorf("members %d and %d have the same id", i, j)
			}
		}
	}
	return nil
}

func (g *WeightedRoundRobinGroup) calculateTotalVotingPower() {
	var totalVotingPower uint64 = 0
	for _, m := range g.members {
		totalVotingPower += uint64(m.Weight())
	}
	g.totalVotingPower = totalVotingPower
}
