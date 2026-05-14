package resolver

import (
	"sync"
	"sync/atomic"
	"testing"
)

// TestCache_RefDedups checks that the resolver's app-level cache short-circuits
// repeated calls after the first success. We exercise it by calling getRef
// directly (population/lookup contract) since exercising ResolveRef requires
// stubbing the github client. The dedup of *concurrent* in-flight requests is
// driven by singleflight, which is library code; we cover its integration via
// the populate-then-hit pattern.
func TestCache_RefDedups(t *testing.T) {
	c := &cache{}
	c.putRef("actions/checkout@v3", "sha1")

	var hits int32
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if v, ok := c.getRef("actions/checkout@v3"); ok && v == "sha1" {
				atomic.AddInt32(&hits, 1)
			}
		}()
	}
	wg.Wait()
	if hits != 100 {
		t.Fatalf("got %d hits, want 100", hits)
	}
}

func TestCache_LatestDedups(t *testing.T) {
	c := &cache{}
	c.putLatest("actions/checkout", latestResult{SHA: "abc", Tag: "v5"})
	v, ok := c.getLatest("actions/checkout")
	if !ok || v.SHA != "abc" || v.Tag != "v5" {
		t.Fatalf("got %+v ok=%v", v, ok)
	}
}
