package action

import (
	"regexp"
	"strings"
)

var shaRegex = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

// Ref is a parsed `uses:` action reference.
type Ref struct {
	Owner   string // "actions"
	Repo    string // "cache"
	Subpath string // "save" for actions/cache/save@v3 (no leading slash, empty if none)
	Ref     string // raw ref as written (branch/tag/SHA)
	IsSHA   bool   // 40-char hex
	// Comment fields are only populated when the source line had `# ...`.
	SrcRef      string // first whitespace-delimited token after `#`
	CommentTail string // remainder of comment after SrcRef (preserved as-is incl. leading space)
}

// Action returns "owner/repo" or "owner/repo/subpath".
func (r Ref) Action() string {
	if r.Subpath == "" {
		return r.Owner + "/" + r.Repo
	}
	return r.Owner + "/" + r.Repo + "/" + r.Subpath
}

// IsSHA reports whether s looks like a 40-char hex commit SHA.
func IsSHA(s string) bool {
	return shaRegex.MatchString(s)
}

// ParseUsesValue parses the value portion of a `uses:` directive
// (the bit after `uses:`), e.g. "actions/checkout@v3 # v3 extra".
// Returns ok=false for non-pinnable values (docker://, ./local, malformed).
func ParseUsesValue(action, ref, srcRef, commentTail string) (Ref, bool) {
	if action == "" || ref == "" {
		return Ref{}, false
	}
	if strings.HasPrefix(action, "./") || strings.HasPrefix(action, "../") || action == "." {
		return Ref{}, false
	}
	if strings.Contains(action, "://") {
		return Ref{}, false
	}
	parts := strings.SplitN(action, "/", 3)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return Ref{}, false
	}
	r := Ref{
		Owner:       parts[0],
		Repo:        parts[1],
		Ref:         ref,
		IsSHA:       IsSHA(ref),
		SrcRef:      srcRef,
		CommentTail: commentTail,
	}
	if len(parts) == 3 {
		r.Subpath = parts[2]
	}
	return r, true
}
