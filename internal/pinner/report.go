package pinner

import (
	"fmt"
	"io"
	"sync"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/wallrat/ghap/internal/action"
	"github.com/wallrat/ghap/internal/workflow"
	"golang.org/x/sync/errgroup"
)

// PrintChange writes a one-line, machine-readable change record to w.
// Format: `<kind> file=<path> line=<n> action=<a> before=<ref> after=<ref>`.
func PrintChange(w io.Writer, c Change) {
	fmt.Fprintf(w, "%-6s file=%s line=%d action=%s before=%s after=%s\n",
		c.Kind, c.File, c.LineIndex+1, c.Action, short(c.BeforeRef), short(c.AfterRef))
}

func short(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

// TableRow is one row in the inspection table.
type TableRow struct {
	Workflow string
	Action   string
	Current  string // ref as written + optional `# srcref` annotation
	Pin      string // SHA the current ref resolves to, or "-"
	Latest   string // SHA of latest release tag, or "-" / error
	// CurrentSHA is the file's literal SHA when the action is already pinned
	// (empty otherwise). Used to color-code the Pin column.
	CurrentSHA string
	// PinSHA / LatestSHA hold the full unformatted SHAs (when known). Used by
	// the renderer to color-code cells without re-parsing strings.
	PinSHA    string
	LatestSHA string
	// LineIndex is the 0-based line in Workflow where this `uses:` lives.
	LineIndex int
	// PinErr / LatestErr carry the underlying resolver errors (nil on success or
	// when the column was not queried). Surfaced to callers for -v output.
	PinErr    error
	LatestErr error
}

// BuildTableRows enumerates every `uses:` match across paths and resolves the
// "pin" and "latest" columns concurrently. Errors per row become "<err>".
func BuildTableRows(paths []string, res Resolver, concurrency int) ([]TableRow, error) {
	if concurrency <= 0 {
		concurrency = 8
	}
	type job struct {
		path  string
		match workflow.UsesMatch
		row   TableRow
	}
	var jobs []*job
	for _, p := range paths {
		f, err := workflow.Read(p)
		if err != nil {
			return nil, err
		}
		for _, m := range f.FindAllUses() {
			var cur string
			switch {
			case m.Action.IsSHA && m.Action.SrcRef != "":
				cur = short(m.Action.Ref) + " (" + m.Action.SrcRef + ")"
			case m.Action.IsSHA:
				cur = short(m.Action.Ref)
			default:
				cur = m.Action.Ref
			}
			row := TableRow{
				Workflow:  p,
				Action:    m.Action.Action(),
				Current:   cur,
				Pin:       "-",
				Latest:    "-",
				LineIndex: m.LineIndex,
			}
			if m.Action.IsSHA {
				row.CurrentSHA = m.Action.Ref
			}
			// Orphan pinned line (SHA without `# ref` comment): nothing to
			// resolve — the file's own SHA *is* what we'd pin to.
			if m.Action.IsSHA && m.Action.SrcRef == "" {
				row.Pin = short(m.Action.Ref)
				row.PinSHA = m.Action.Ref
			}
			jobs = append(jobs, &job{path: p, match: m, row: row})
		}
	}

	g := new(errgroup.Group)
	g.SetLimit(concurrency)
	var mu sync.Mutex
	for _, j := range jobs {
		g.Go(func() error {
			a := j.match.Action
			// Pin column: only meaningful when not already SHA, or when there's
			// a source ref we'd refresh.
			if !a.IsSHA {
				sha, err := res.ResolveRef(a.Owner, a.Repo, a.Ref)
				mu.Lock()
				if err != nil {
					j.row.Pin = "<err>"
					j.row.PinErr = err
				} else {
					j.row.Pin = short(sha)
					j.row.PinSHA = sha
				}
				mu.Unlock()
			} else if a.SrcRef != "" {
				sha, err := res.ResolveRef(a.Owner, a.Repo, a.SrcRef)
				mu.Lock()
				if err != nil {
					j.row.Pin = "<err>"
					j.row.PinErr = err
				} else {
					j.row.Pin = short(sha)
					j.row.PinSHA = sha
				}
				mu.Unlock()
			}
			latest, tag, err := res.LatestReleaseSHA(a.Owner, a.Repo)
			mu.Lock()
			if err != nil {
				j.row.Latest = "<err>"
				j.row.LatestErr = err
			} else {
				j.row.Latest = short(latest) + " (" + tag + ")"
				j.row.LatestSHA = latest
			}
			mu.Unlock()
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	out := make([]TableRow, len(jobs))
	for i, j := range jobs {
		out[i] = j.row
	}
	return out, nil
}

// ChangeRow describes one applied (or planned, in dry-run) change for the
// post-mutation summary table.
type ChangeRow struct {
	Workflow  string
	Action    string
	Before    string
	After     string
	Latest    string
	AfterSHA  string
	LatestSHA string
	LatestErr error
}

// BuildChangeRows builds one row per change in result, fetching the latest
// release SHA for each unique action via the resolver (deduped through the
// resolver's singleflight + cache).
func BuildChangeRows(result *Result, res Resolver, concurrency int) ([]ChangeRow, error) {
	if concurrency <= 0 {
		concurrency = 8
	}
	type pending struct {
		row   ChangeRow
		owner string
		repo  string
	}
	var rows []*pending
	for _, fp := range result.Files {
		for _, c := range fp.Changes {
			a := c.match.Action
			rows = append(rows, &pending{
				owner: a.Owner,
				repo:  a.Repo,
				row: ChangeRow{
					Workflow: c.File,
					Action:   c.Action,
					Before:   formatRef(c.BeforeRef, c.BeforeSrcRef),
					After:    formatRef(c.AfterRef, c.AfterSrcRef),
					AfterSHA: c.AfterRef,
				},
			})
		}
	}

	g := new(errgroup.Group)
	g.SetLimit(concurrency)
	var mu sync.Mutex
	for _, p := range rows {
		g.Go(func() error {
			sha, tag, err := res.LatestReleaseSHA(p.owner, p.repo)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				p.row.Latest = "<err>"
				p.row.LatestErr = err
				return nil
			}
			p.row.Latest = short(sha) + " (" + tag + ")"
			p.row.LatestSHA = sha
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	out := make([]ChangeRow, len(rows))
	for i, p := range rows {
		out[i] = p.row
	}
	return out, nil
}

// formatRef renders a ref as either `<short-sha> (<srcRef>)`, `<short-sha>`,
// or the raw ref string, depending on whether ref is a SHA and srcRef is set.
func formatRef(ref, srcRef string) string {
	if ref == "" {
		return "-"
	}
	if action.IsSHA(ref) {
		if srcRef != "" {
			return short(ref) + " (" + srcRef + ")"
		}
		return short(ref)
	}
	return ref
}

// RenderChangeTable writes one table per workflow listing every applied change
// (Action, Before, After, Latest). The Latest cell is green when After matches
// Latest and red otherwise (same rule as the default inspection table).
func RenderChangeTable(w io.Writer, rows []ChangeRow) {
	if len(rows) == 0 {
		return
	}
	header := lipgloss.NewStyle().Bold(true).Underline(true)
	green := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	red := lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	const latestCol = 3 // 0=Action, 1=Before, 2=After, 3=Latest

	var order []string
	groups := map[string][]ChangeRow{}
	for _, r := range rows {
		if _, ok := groups[r.Workflow]; !ok {
			order = append(order, r.Workflow)
		}
		groups[r.Workflow] = append(groups[r.Workflow], r)
	}

	for i, wf := range order {
		if i > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintln(w, header.Render(wf))
		grp := groups[wf]
		t := table.New().
			Border(lipgloss.NormalBorder()).
			Headers("Action", "Before", "After", "Latest").
			StyleFunc(func(row, col int) lipgloss.Style {
				if row < 0 || row >= len(grp) || col != latestCol {
					return lipgloss.NewStyle()
				}
				r := grp[row]
				if r.LatestSHA == "" || r.AfterSHA == "" {
					return lipgloss.NewStyle()
				}
				if r.LatestSHA == r.AfterSHA {
					return green
				}
				return red
			})
		for _, r := range grp {
			t.Row(r.Action, r.Before, r.After, r.Latest)
		}
		fmt.Fprintln(w, t.Render())
	}
}

// RenderTable writes one table per workflow to w, with the workflow path as
// a header above each table. Workflow order matches the input row order.
func RenderTable(w io.Writer, rows []TableRow) {
	header := lipgloss.NewStyle().Bold(true).Underline(true)

	var order []string
	groups := map[string][]TableRow{}
	for _, r := range rows {
		if _, ok := groups[r.Workflow]; !ok {
			order = append(order, r.Workflow)
		}
		groups[r.Workflow] = append(groups[r.Workflow], r)
	}

	green := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	red := lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	const (
		pinCol    = 2
		latestCol = 3 // 0=Action, 1=Current, 2=Pin, 3=Latest
	)

	for i, wf := range order {
		if i > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintln(w, header.Render(wf))
		grp := groups[wf]
		t := table.New().
			Border(lipgloss.NormalBorder()).
			Headers("Action", "Current", "Pin", "Latest").
			StyleFunc(func(row, col int) lipgloss.Style {
				if row < 0 || row >= len(grp) {
					return lipgloss.NewStyle()
				}
				r := grp[row]
				switch col {
				case pinCol:
					if r.CurrentSHA != "" && r.PinSHA != "" && r.CurrentSHA == r.PinSHA {
						return green
					}
				case latestCol:
					if r.LatestSHA != "" && r.PinSHA != "" {
						if r.LatestSHA == r.PinSHA {
							return green
						}
						return red
					}
				}
				return lipgloss.NewStyle()
			})
		for _, r := range grp {
			t.Row(r.Action, r.Current, r.Pin, r.Latest)
		}
		fmt.Fprintln(w, t.Render())
	}
}
