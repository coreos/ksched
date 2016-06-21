// Tests for resourcestatus.go

package resourcestatus

import "testing"

// Test setter and getter for heartbeat
func TestSetGetHeartBeat(t *testing.T) {
	cases := []struct {
		in, want uint64
	}{
		{10, 10},
		{1234, 1234},
	}
	rs := new(ResourceStatus)
	for _, c := range cases {
		rs.SetLastHeartbeat(c.in)
		got := rs.LastHeartbeat()
		if got != c.want {
			t.Errorf("SetLastHeartbeat(%d), LastHeartbeat() == %d, want %d", c.in, got, c.want)
		}
	}

}
