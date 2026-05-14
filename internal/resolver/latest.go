package resolver

import (
	"fmt"

	"github.com/google/go-github/v72/github"
)

// LatestReleaseSHA returns the SHA pointed at by the most recent tag on
// owner/repo. Falls back to the default branch HEAD if the repo has no tags.
// Successful results are memoized; concurrent identical lookups share one
// API call via singleflight.
//
// We use ListTags because not every action publishes formal "releases" — many
// just tag.
func (r *Resolver) LatestReleaseSHA(owner, repo string) (sha string, srcTag string, err error) {
	key := owner + "/" + repo
	if v, ok := r.cache.getLatest(key); ok {
		return v.SHA, v.Tag, nil
	}
	v, err, _ := r.cache.sf.Do("latest:"+key, func() (any, error) {
		if cached, ok := r.cache.getLatest(key); ok {
			return cached, nil
		}
		s, t, err := r.latestReleaseSHAUncached(owner, repo)
		if err == nil {
			r.cache.putLatest(key, latestResult{SHA: s, Tag: t})
		}
		return latestResult{SHA: s, Tag: t}, err
	})
	if err != nil {
		return "", "", err
	}
	lr := v.(latestResult)
	return lr.SHA, lr.Tag, nil
}

func (r *Resolver) latestReleaseSHAUncached(owner, repo string) (sha string, srcTag string, err error) {
	ctx := ctxBg()

	tags, _, err := r.GH.Repositories.ListTags(ctx, owner, repo, &github.ListOptions{PerPage: 1})
	if err != nil {
		return "", "", r.wrapErr(err)
	}
	if len(tags) > 0 {
		t := tags[0]
		name := t.GetName()
		// ListTags returns the commit the tag points at directly, which for
		// annotated tags is already the dereferenced commit. Good enough.
		if c := t.GetCommit(); c != nil && c.GetSHA() != "" {
			return c.GetSHA(), name, nil
		}
		// Fallback: resolve manually.
		s, rerr := r.ResolveRef(owner, repo, name)
		return s, name, rerr
	}

	repoInfo, _, err := r.GH.Repositories.Get(ctx, owner, repo)
	if err != nil {
		return "", "", r.wrapErr(err)
	}
	def := repoInfo.GetDefaultBranch()
	if def == "" {
		return "", "", fmt.Errorf("%s/%s has no tags and no default branch", owner, repo)
	}
	ref, _, err := r.GH.Git.GetRef(ctx, owner, repo, "refs/heads/"+def)
	if err != nil {
		return "", "", r.wrapErr(err)
	}
	if ref.Object == nil {
		return "", "", fmt.Errorf("%s/%s: default branch %s has no object", owner, repo, def)
	}
	return ref.Object.GetSHA(), def, nil
}
