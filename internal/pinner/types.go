package pinner

import (
	"github.com/wallrat/ghap/internal/workflow"
)

// Kind identifies the rule that produced a change.
type Kind string

const (
	KindPin    Kind = "pin"    // unpinned → SHA, add `# <ref>` comment
	KindUpdate Kind = "update" // already-pinned re-resolved from `# <srcRef>`
	KindLatest Kind = "latest" // pinned → latest tag SHA + latest tag comment, drop comment tail
)

// Change is one pending rewrite of a single `uses:` directive.
type Change struct {
	File      string
	LineIndex int
	Kind      Kind

	Action       string // owner/repo[/sub]
	BeforeRef    string
	AfterRef     string
	BeforeSrcRef string // comment ref before
	AfterSrcRef  string // comment ref after ("" = drop comment)
	AfterTail    string // comment tail after (preserved or "")

	// Internal use during apply: the original match's location.
	match workflow.UsesMatch
}

// SkipReason explains why an action was skipped. Used for table output and
// verbose logging.
type SkipReason string

const (
	SkipAlreadyPinned SkipReason = "already-pinned" // pin: ref is SHA
	SkipNotPinned     SkipReason = "not-pinned"     // update: ref is not SHA
	SkipUnchanged     SkipReason = "unchanged"      // resolved SHA equals existing
	SkipResolveFailed SkipReason = "resolve-failed" // GH API error (non-fatal)
)

// Skipped records why a particular match was not converted to a Change.
type Skipped struct {
	File      string
	LineIndex int
	Action    string
	Ref       string
	Reason    SkipReason
	Err       error
}
