package resolver

import (
	"sync"

	"golang.org/x/sync/singleflight"
)

// cache memoizes successful resolver lookups and uses singleflight to dedupe
// concurrent identical lookups (so a workflow with 6× `actions/checkout@v3`
// makes 1 API call, not 6, even when the calls race).
//
// Only successes are cached. Errors (including rate limits) flow through
// untouched so transient failures can be retried.
type cache struct {
	sf       singleflight.Group
	refs     sync.Map // key "owner/repo@ref" -> string SHA
	latest   sync.Map // key "owner/repo"     -> latestResult
}

type latestResult struct {
	SHA string
	Tag string
}

func (c *cache) getRef(key string) (string, bool) {
	v, ok := c.refs.Load(key)
	if !ok {
		return "", false
	}
	return v.(string), true
}

func (c *cache) putRef(key, sha string) {
	c.refs.Store(key, sha)
}

func (c *cache) getLatest(key string) (latestResult, bool) {
	v, ok := c.latest.Load(key)
	if !ok {
		return latestResult{}, false
	}
	return v.(latestResult), true
}

func (c *cache) putLatest(key string, r latestResult) {
	c.latest.Store(key, r)
}
