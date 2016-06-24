package util

import (
	"hash/fnv"
	"strconv"

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
