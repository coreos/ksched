package idgenerator

// Utilility to return a randomized or increasing sequence of unique ids starting from 1
// Ids can also be reused

import (
	"math/rand"
	"time"

	"github.com/coreos/ksched/pkg/util/queue"
)

type IDGen interface {
	NextID() uint64
	RecycleID(uint64)
}

type idGen struct {
	nextID uint64
	// Queue storing the ids that we've previously removed.
	unusedIDs queue.FIFO
	// NOTE: Randomized mode is only unique for the first run
	// TODO: Randomized mode is probably useless, remove it later
	RandomizeIDs bool
}

func New(randomizeIDs bool) IDGen {
	ig := &idGen{
		nextID:       1,
		unusedIDs:    queue.NewFIFO(),
		RandomizeIDs: randomizeIDs,
	}
	if randomizeIDs {
		ig.populateUnusedIds(50)
	}
	return ig
}

// Returns the nextID to assign to a node
func (ig *idGen) NextID() uint64 {
	if ig.RandomizeIDs {
		if ig.unusedIDs.IsEmpty() {
			ig.populateUnusedIds(ig.nextID * 2)
		}
		return ig.unusedIDs.Pop().(uint64)
	}
	if ig.unusedIDs.IsEmpty() {
		newID := ig.nextID
		ig.nextID++
		return newID
	}
	return ig.unusedIDs.Pop().(uint64)
}

func (ig *idGen) RecycleID(oldID uint64) {
	ig.unusedIDs.Push(oldID)
}

// Called if RandomizeIDs is true to generate a random shuffle of ids
func (ig *idGen) populateUnusedIds(newNextID uint64) {
	t := time.Now().UnixNano()
	r := rand.New(rand.NewSource(t))
	ids := make([]uint64, 0)
	for i := ig.nextID; i < newNextID; i++ {
		ids = append(ids, i)
	}
	// Fisher-Yates shuffle
	for i := range ids {
		j := r.Intn(i + 1)
		ids[i], ids[j] = ids[j], ids[i]
	}
	for i := range ids {
		ig.unusedIDs.Push(ids[i])
	}
	ig.nextID = newNextID
}
