package idgenerator

import (
	"fmt"
	"testing"
)

func TestIDs(t *testing.T) {
	idgen := New(true)
	for i := 0; i < 10; i++ {
		fmt.Printf("%d\n", idgen.NextID())
	}

	idgen.RecycleID(7)
	for i := 0; i < 10; i++ {
		fmt.Printf("%d\n", idgen.NextID())
	}
}
