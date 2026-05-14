package pinner

import (
	"errors"
	"sync"

	"github.com/wallrat/ghap/internal/action"
	"github.com/wallrat/ghap/internal/resolver"
	"github.com/wallrat/ghap/internal/workflow"
	"golang.org/x/sync/errgroup"
)

// Mode controls which subcommand's rules apply when building a plan.
type Mode int

const (
	ModePin    Mode = iota // pin: unpinned → SHA + `# <ref>` comment
	ModeUpdate             // update: pinned → re-resolve via comment (or latest if no comment)
	ModeLatest             // update --latest: pinned → latest tag SHA + latest tag comment, drop comment tail
)

// FilePlan is the result of planning one workflow file.
type FilePlan struct {
	File    *workflow.File
	Changes []Change
	Skipped []Skipped
}

// Result is the aggregate of all per-file plans.
type Result struct {
	Files []*FilePlan
}

// AllChanges flattens changes across files in order.
func (r *Result) AllChanges() []Change {
	var out []Change
	for _, f := range r.Files {
		out = append(out, f.Changes...)
	}
	return out
}

type plannedTask struct {
	fp     *FilePlan
	match  workflow.UsesMatch
	change *Change
	skip   *Skipped
}

// Plan walks each workflow file, classifies every `uses:` match according to
// `mode`, and resolves SHAs concurrently using `res`. Non-rate-limit resolver
// errors become Skipped entries; a rate-limit error aborts.
func Plan(paths []string, mode Mode, res Resolver, concurrency int) (*Result, error) {
	if concurrency <= 0 {
		concurrency = 8
	}
	result := &Result{}
	var tasks []*plannedTask

	for _, p := range paths {
		f, err := workflow.Read(p)
		if err != nil {
			return nil, err
		}
		fp := &FilePlan{File: f}
		result.Files = append(result.Files, fp)
		for _, m := range f.FindAllUses() {
			t := &plannedTask{fp: fp, match: m}
			classify(t, mode)
			tasks = append(tasks, t)
		}
	}

	g := new(errgroup.Group)
	g.SetLimit(concurrency)
	var mu sync.Mutex
	for _, t := range tasks {
		if t.change == nil { // already a skip
			continue
		}
		g.Go(func() error {
			err := resolveChange(t.change, t.match.Action, res)
			if err == nil {
				return nil
			}
			var rl *resolver.RateLimitedError
			if errors.As(err, &rl) {
				return err // abort
			}
			mu.Lock()
			t.skip = &Skipped{
				File:      t.fp.File.Path,
				LineIndex: t.match.LineIndex,
				Action:    t.match.Action.Action(),
				Ref:       t.match.Action.Ref,
				Reason:    SkipResolveFailed,
				Err:       err,
			}
			t.change = nil
			mu.Unlock()
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return result, err
	}

	for _, t := range tasks {
		switch {
		case t.change != nil:
			// Drop no-op changes (SHA + comment ref both unchanged).
			if t.change.BeforeRef == t.change.AfterRef &&
				t.change.BeforeSrcRef == t.change.AfterSrcRef {
				t.fp.Skipped = append(t.fp.Skipped, Skipped{
					File:      t.fp.File.Path,
					LineIndex: t.match.LineIndex,
					Action:    t.match.Action.Action(),
					Ref:       t.match.Action.Ref,
					Reason:    SkipUnchanged,
				})
				continue
			}
			t.fp.Changes = append(t.fp.Changes, *t.change)
		case t.skip != nil:
			t.fp.Skipped = append(t.fp.Skipped, *t.skip)
		}
	}

	return result, nil
}

func classify(t *plannedTask, mode Mode) {
	a := t.match.Action
	switch mode {
	case ModePin:
		if a.IsSHA {
			t.skip = &Skipped{
				File: t.fp.File.Path, LineIndex: t.match.LineIndex,
				Action: a.Action(), Ref: a.Ref, Reason: SkipAlreadyPinned,
			}
			return
		}
		t.change = &Change{
			File: t.fp.File.Path, LineIndex: t.match.LineIndex, Kind: KindPin,
			Action:       a.Action(),
			BeforeRef:    a.Ref,
			BeforeSrcRef: a.SrcRef,
			AfterSrcRef:  a.Ref,
			AfterTail:    pinCommentTail(a),
			match:        t.match,
		}
	case ModeUpdate:
		c := &Change{
			File: t.fp.File.Path, LineIndex: t.match.LineIndex,
			Action:       a.Action(),
			BeforeRef:    a.Ref,
			BeforeSrcRef: a.SrcRef,
			AfterTail:    a.CommentTail,
			match:        t.match,
		}
		switch {
		case !a.IsSHA:
			// Unpinned `@ref` — pin it: resolve the ref and add `# ref` comment.
			c.Kind = KindPin
			c.AfterSrcRef = a.Ref
			c.AfterTail = pinCommentTail(a)
		case a.SrcRef != "":
			// Pinned with source comment — re-resolve via the comment.
			c.Kind = KindUpdate
			c.AfterSrcRef = a.SrcRef
		default:
			// Orphan pinned line — bump to latest; resolver sets AfterSrcRef.
			c.Kind = KindLatest
			c.AfterTail = ""
		}
		t.change = c
	case ModeLatest:
		// Bump every action (pinned or not) to the latest release commit.
		t.change = &Change{
			File: t.fp.File.Path, LineIndex: t.match.LineIndex, Kind: KindLatest,
			Action:       a.Action(),
			BeforeRef:    a.Ref,
			BeforeSrcRef: a.SrcRef,
			// AfterSrcRef filled in by resolver with the resolved latest tag.
			AfterTail: "", // discard stale user comment content
			match:     t.match,
		}
	}
}

func pinCommentTail(a action.Ref) string {
	if a.SrcRef == "" {
		return a.CommentTail
	}
	return " " + a.SrcRef + a.CommentTail
}

func resolveChange(c *Change, a action.Ref, res Resolver) error {
	switch c.Kind {
	case KindPin:
		sha, err := res.ResolveRef(a.Owner, a.Repo, a.Ref)
		if err != nil {
			return err
		}
		c.AfterRef = sha
		return nil
	case KindUpdate:
		sha, err := res.ResolveRef(a.Owner, a.Repo, a.SrcRef)
		if err != nil {
			return err
		}
		c.AfterRef = sha
		return nil
	case KindLatest:
		sha, tag, err := res.LatestReleaseSHA(a.Owner, a.Repo)
		if err != nil {
			return err
		}
		c.AfterRef = sha
		c.AfterSrcRef = tag // annotate with the latest tag (or default-branch name)
		return nil
	}
	return nil
}
