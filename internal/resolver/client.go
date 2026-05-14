package resolver

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/go-github/v72/github"
	"github.com/gregjones/httpcache"
)

// Resolver wraps a GitHub client and tracks whether it is authenticated.
// It memoizes successful lookups in-process and uses singleflight to dedupe
// concurrent identical lookups.
type Resolver struct {
	GH            *github.Client
	Authenticated bool
	cache         cache
}

// New builds a resolver. If token is empty, the client is unauthenticated.
func New(token string) *Resolver {
	gh := github.NewClient(httpcache.NewMemoryCacheTransport().Client())
	if token != "" {
		gh = gh.WithAuthToken(token)
	}
	return &Resolver{GH: gh, Authenticated: token != ""}
}

// RateLimitedError is returned for any GitHub call that hit a rate / abuse
// limit. The Authenticated bit lets callers tailor the message.
type RateLimitedError struct {
	Authenticated bool
	Wrapped       error
}

func (e *RateLimitedError) Error() string {
	if e.Authenticated {
		return fmt.Sprintf("github rate limit hit (authenticated, 5000/hr cap): %v", e.Wrapped)
	}
	return fmt.Sprintf("github rate limit hit (anonymous, 60/hr cap) — set GITHUB_TOKEN or pass --token for 5000/hr: %v", e.Wrapped)
}

func (e *RateLimitedError) Unwrap() error { return e.Wrapped }

// wrapErr converts go-github rate-limit errors to our typed error.
func (r *Resolver) wrapErr(err error) error {
	if err == nil {
		return nil
	}
	var rl *github.RateLimitError
	var ab *github.AbuseRateLimitError
	if errors.As(err, &rl) || errors.As(err, &ab) {
		return &RateLimitedError{Authenticated: r.Authenticated, Wrapped: err}
	}
	return err
}

// ctxBg returns a fresh background context. Centralized so we can add timeouts
// later without touching every call site.
func ctxBg() context.Context { return context.Background() }
