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

// Random number generator
var randGen *rand.Rand

// See the rng based on current time
func init() {
	t := time.Now().UnixNano()
	randGen = rand.New(rand.NewSource(t))
}

// Seed the rng to generate deterministic IDs for testing purposes
func SeedRNGWithInt(seed int64) {
	randGen = rand.New(rand.NewSource(seed))
}

func SeedRNGWithString(seed string) {
	SeedRNGWithInt(int64(hashFNV(seed)))
}

// Returns a 64 bit int from fnv hash of the string
func hashFNV(s string) uint64 {
	h := fnv.New64()
	h.Write([]byte(s))
	return h.Sum64()
}

// Generate a uint64 random number
func RandUint64() uint64 {
	// Using two calls to uint32 since there is no rand.uint64
	return uint64(randGen.Uint32()) + uint64(randGen.Uint32())
}

// ID generators
// NOTE: For GenerateRootResourceID() just use this func
func GenerateResourceID() types.ResourceID {
	return types.ResourceID(RandUint64())
}

func GenerateJobID() types.JobID {
	return types.JobID(RandUint64())
}

// For GenerateRootTaskID() just use this func
func GenerateTaskID() types.TaskID {
	return types.TaskID(RandUint64())
}
