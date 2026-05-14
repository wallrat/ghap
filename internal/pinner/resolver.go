package pinner

// Resolver is the surface area the pinner needs from internal/resolver.
// Defined as an interface so plan logic can be tested with fakes.
type Resolver interface {
	ResolveRef(owner, repo, ref string) (sha string, err error)
	LatestReleaseSHA(owner, repo string) (sha, srcTag string, err error)
}
