package util

import (
	"hash/fnv"
	"math/rand"
	"strconv"
	"time"

	"github.com/coreos/ksched/pkg/types"
)

func HashBytesToEquivClass(b []byte) types.EquivClass {
	h := fnv.New64()
	h.Write(b)
	return types.EquivClass(h.Sum64())
}

func ResourceIDFromString(s string) (types.ResourceID, error) {
	i, err := strconv.ParseUint(s, 10, 64)
	return types.ResourceID(i), err
}

func MustJobIDFromString(s string) types.JobID {
	i, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		panic(err)
	}
	return types.JobID(i)
}

func MustResourceIDFromString(s string) types.ResourceID {
	id, err := ResourceIDFromString(s)
	if err != nil {
		panic(err)
	}
	return id
}

// NOTE: Just using a single rng for generating Resource, Job or Task IDs
// instead of separate rngs for each ID
// Also just seeding it once, as opposed to having the option for a custom seed
// on every ID generation

// Random number generator
var randGen *rand.Rand

// See the rng based on current time
func init() {
	t := time.Now().UnixNano()
	randGen = rand.New(rand.NewSource(t))
}

// Generate a uint64 random number
func randUint64() uint64 {
	// Using two calls to uint32 since there is no rand.uint64
	return uint64(randGen.Uint32()) + uint64(randGen.Uint32())
}

// ID generators
// NOTE: For GenerateRootResourceID() just use this func
func GenerateResourceID() types.ResourceID {
	return types.ResourceID(randUint64())
}

func GenerateJobID() types.JobID {
	return types.JobID(randUint64())
}

func GenerateTaskID() types.TaskID {
	return types.TaskID(randUint64())
}
