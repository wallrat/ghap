package resolver

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/google/go-github/v72/github"
)

// ResolveRef returns the commit SHA that `ref` (branch or tag) points at for
// owner/repo. Successful results are memoized; concurrent identical lookups
// share a single API call via singleflight.
func (r *Resolver) ResolveRef(owner, repo, ref string) (string, error) {
	key := owner + "/" + repo + "@" + ref
	if sha, ok := r.cache.getRef(key); ok {
		return sha, nil
	}
	v, err, _ := r.cache.sf.Do("ref:"+key, func() (any, error) {
		if sha, ok := r.cache.getRef(key); ok {
			return sha, nil
		}
		sha, err := r.resolveRefUncached(owner, repo, ref)
		if err == nil {
			r.cache.putRef(key, sha)
		}
		return sha, err
	})
	if err != nil {
		return "", err
	}
	return v.(string), nil
}

// resolveRefUncached resolves the ref the same way GitHub Actions does for
// ambiguous names: tags take precedence over branches. Annotated tags are
// dereferenced to their target commit.
func (r *Resolver) resolveRefUncached(owner, repo, ref string) (string, error) {
	ctx := ctxBg()

	tref, _, err := r.GH.Git.GetRef(ctx, owner, repo, "refs/tags/"+ref)
	if err == nil && tref.Object != nil {
		sha := tref.Object.GetSHA()
		if tref.Object.GetType() == "tag" {
			tagObj, _, terr := r.GH.Git.GetTag(ctx, owner, repo, sha)
			if terr == nil && tagObj.Object != nil {
				return tagObj.Object.GetSHA(), nil
			}
			if wrapped := r.wrapErr(terr); wrapped != nil {
				return "", wrapped
			}
		}
		return sha, nil
	}
	if wrapped := r.wrapErr(err); wrapped != nil {
		var rl *RateLimitedError
		if errors.As(wrapped, &rl) {
			return "", wrapped
		}
		if !isNotFound(wrapped) {
			return "", wrapped
		}
	}

	bref, _, err := r.GH.Git.GetRef(ctx, owner, repo, "refs/heads/"+ref)
	if err == nil && bref.Object != nil {
		return bref.Object.GetSHA(), nil
	}
	if wrapped := r.wrapErr(err); wrapped != nil {
		var rl *RateLimitedError
		if errors.As(wrapped, &rl) {
			return "", wrapped
		}
		if !isNotFound(wrapped) {
			return "", wrapped
		}
	}

	return "", fmt.Errorf("could not resolve %s/%s@%s: not a branch or tag", owner, repo, ref)
}

func isNotFound(err error) bool {
	var ghErr *github.ErrorResponse
	return errors.As(err, &ghErr) && ghErr.Response != nil && ghErr.Response.StatusCode == http.StatusNotFound
}
