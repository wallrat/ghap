package pinner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeResolver struct {
	resolve   map[string]string // "owner/repo@ref" -> sha
	latest    map[string]string // "owner/repo" -> sha
	latestTag map[string]string // "owner/repo" -> tag name
}

func (f *fakeResolver) ResolveRef(owner, repo, ref string) (string, error) {
	if s, ok := f.resolve[owner+"/"+repo+"@"+ref]; ok {
		return s, nil
	}
	return "", os.ErrNotExist
}

func (f *fakeResolver) LatestReleaseSHA(owner, repo string) (string, string, error) {
	s, ok := f.latest[owner+"/"+repo]
	if !ok {
		return "", "", os.ErrNotExist
	}
	return s, f.latestTag[owner+"/"+repo], nil
}

func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "wf.yml")
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

const baseWF = `name: test
on: push
jobs:
  j:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-node@v4
      - uses: actions/cache@%s
      - uses: actions/upload-artifact@%s
`

const sha40 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const sha40b = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
const sha40c = "cccccccccccccccccccccccccccccccccccccccc"
const sha40d = "dddddddddddddddddddddddddddddddddddddddd"

func TestPlan_Pin(t *testing.T) {
	body := strings.Replace(baseWF, "%s", sha40+" # v3", 1)
	body = strings.Replace(body, "%s", "v3", 1)
	p := writeTempFile(t, body)

	fake := &fakeResolver{
		resolve: map[string]string{
			"actions/checkout@v3":        sha40,
			"actions/setup-node@v4":      sha40b,
			"actions/upload-artifact@v3": sha40d,
		},
	}
	r, err := Plan([]string{p}, ModePin, fake, 4)
	if err != nil {
		t.Fatal(err)
	}
	got := r.AllChanges()
	// Expect 3 pin changes (one line is already pinned: actions/cache).
	if len(got) != 3 {
		t.Fatalf("got %d changes\n%+v", len(got), got)
	}
	for _, c := range got {
		if c.Kind != KindPin {
			t.Errorf("kind=%v want pin", c.Kind)
		}
		if c.AfterSrcRef == "" {
			t.Errorf("pin should write a source comment, got empty for %s", c.Action)
		}
	}
}

func TestPlan_Update_HandlesUnpinnedAndPinnedTogether(t *testing.T) {
	// Lines 1-2 unpinned (checkout@v3, setup-node@v4); 3-4 pinned with `# v3`.
	body := strings.Replace(baseWF, "%s", sha40+" # v3", 1) // cache
	body = strings.Replace(body, "%s", sha40b+" # v3", 1)   // upload-artifact
	p := writeTempFile(t, body)

	fake := &fakeResolver{
		resolve: map[string]string{
			"actions/checkout@v3":        sha40c, // unpinned → pin
			"actions/setup-node@v4":      sha40d, // unpinned → pin
			"actions/cache@v3":           sha40c, // pinned, moved → update
			"actions/upload-artifact@v3": sha40b, // pinned, unchanged → skip
		},
	}
	r, err := Plan([]string{p}, ModeUpdate, fake, 4)
	if err != nil {
		t.Fatal(err)
	}
	got := r.AllChanges()
	// Expect 3 changes: two pins (checkout, setup-node) + one update (cache).
	// upload-artifact is filtered as unchanged.
	if len(got) != 3 {
		t.Fatalf("got %d changes, want 3\n%+v", len(got), got)
	}
	gotKinds := map[string]Kind{}
	for _, c := range got {
		gotKinds[c.Action] = c.Kind
	}
	want := map[string]Kind{
		"actions/checkout":   KindPin,
		"actions/setup-node": KindPin,
		"actions/cache":      KindUpdate,
	}
	for a, wantKind := range want {
		if got := gotKinds[a]; got != wantKind {
			t.Errorf("%s kind=%v want %v", a, got, wantKind)
		}
	}
}

func TestPlan_Update_OrphanFallsBackToLatest(t *testing.T) {
	// Pinned actions but no `# v3` comment → treat each as --latest.
	body := strings.Replace(baseWF, "%s", sha40, 1)
	body = strings.Replace(body, "%s", sha40b, 1)
	// Replace unpinned tag lines too with pinned (no comments).
	body = strings.Replace(body, "actions/checkout@v3", "actions/checkout@"+sha40c, 1)
	body = strings.Replace(body, "actions/setup-node@v4", "actions/setup-node@"+sha40d, 1)
	p := writeTempFile(t, body)

	fake := &fakeResolver{
		latest: map[string]string{
			"actions/checkout":        "1111111111111111111111111111111111111111",
			"actions/setup-node":      "2222222222222222222222222222222222222222",
			"actions/cache":           "3333333333333333333333333333333333333333",
			"actions/upload-artifact": "4444444444444444444444444444444444444444",
		},
		latestTag: map[string]string{
			"actions/checkout":        "v5",
			"actions/setup-node":      "v5",
			"actions/cache":           "v4",
			"actions/upload-artifact": "v4",
		},
	}
	r, err := Plan([]string{p}, ModeUpdate, fake, 4)
	if err != nil {
		t.Fatal(err)
	}
	got := r.AllChanges()
	if len(got) != 4 {
		t.Fatalf("got %d changes, want 4\n%+v", len(got), got)
	}
	wantTag := map[string]string{
		"actions/checkout":        "v5",
		"actions/setup-node":      "v5",
		"actions/cache":           "v4",
		"actions/upload-artifact": "v4",
	}
	for _, c := range got {
		if c.Kind != KindLatest {
			t.Errorf("kind=%v want latest for %s (orphan fallback)", c.Kind, c.Action)
		}
		if c.AfterSrcRef != wantTag[c.Action] {
			t.Errorf("orphan-latest should annotate with resolved tag, got src=%q want %q for %s",
				c.AfterSrcRef, wantTag[c.Action], c.Action)
		}
	}
}

func TestPlan_Latest_WritesTagAndDropsTail(t *testing.T) {
	body := strings.Replace(baseWF, "%s", sha40+" # v3 (verified)", 1)
	body = strings.Replace(body, "%s", sha40b+" # v3", 1)
	// Replace the unpinned tag lines with pinned ones with comments
	body = strings.Replace(body, "actions/checkout@v3", "actions/checkout@"+sha40c+" # v3", 1)
	body = strings.Replace(body, "actions/setup-node@v4", "actions/setup-node@"+sha40d+" # v4", 1)
	p := writeTempFile(t, body)

	fake := &fakeResolver{
		latest: map[string]string{
			"actions/checkout":        "1111111111111111111111111111111111111111",
			"actions/setup-node":      "2222222222222222222222222222222222222222",
			"actions/cache":           "3333333333333333333333333333333333333333",
			"actions/upload-artifact": "4444444444444444444444444444444444444444",
		},
		latestTag: map[string]string{
			"actions/checkout": "v5", "actions/setup-node": "v5",
			"actions/cache": "v4", "actions/upload-artifact": "v4",
		},
	}
	r, err := Plan([]string{p}, ModeLatest, fake, 4)
	if err != nil {
		t.Fatal(err)
	}
	got := r.AllChanges()
	if len(got) != 4 {
		t.Fatalf("got %d changes, want 4", len(got))
	}
	wantTag := map[string]string{
		"actions/checkout":        "v5",
		"actions/setup-node":      "v5",
		"actions/cache":           "v4",
		"actions/upload-artifact": "v4",
	}
	for _, c := range got {
		if c.AfterSrcRef != wantTag[c.Action] {
			t.Errorf("%s AfterSrcRef=%q want %q", c.Action, c.AfterSrcRef, wantTag[c.Action])
		}
	}
	// The resolved latest tag is kept, but stale user-authored comment tails are dropped.
	for _, c := range got {
		if c.AfterTail != "" {
			t.Errorf("%s AfterTail=%q want empty", c.Action, c.AfterTail)
		}
	}
}

func TestApply_PreservesFormatting(t *testing.T) {
	body := strings.Replace(baseWF, "%s", sha40+" # v3", 1)
	body = strings.Replace(body, "%s", "v3", 1)
	p := writeTempFile(t, body)

	fake := &fakeResolver{
		resolve: map[string]string{
			"actions/checkout@v3":        sha40c,
			"actions/setup-node@v4":      sha40d,
			"actions/upload-artifact@v3": sha40b,
		},
	}
	r, err := Plan([]string{p}, ModePin, fake, 4)
	if err != nil {
		t.Fatal(err)
	}
	for _, fp := range r.Files {
		Apply(fp)
		if err := fp.File.Write(); err != nil {
			t.Fatal(err)
		}
	}

	got, _ := os.ReadFile(p)
	out := string(got)
	if !strings.Contains(out, "actions/checkout@"+sha40c+" # v3") {
		t.Errorf("expected checkout pinned with comment, got:\n%s", out)
	}
	if !strings.Contains(out, "actions/upload-artifact@"+sha40b+" # v3") {
		t.Errorf("expected upload-artifact pinned, got:\n%s", out)
	}
	// Pre-existing pinned line should be untouched.
	if !strings.Contains(out, "actions/cache@"+sha40+" # v3") {
		t.Errorf("expected cache line untouched, got:\n%s", out)
	}
	// Indentation preserved on rewritten lines.
	if !strings.Contains(out, "      - uses: actions/checkout@") {
		t.Errorf("indentation lost on checkout line")
	}
}

func TestApply_PreservesQuotedUses(t *testing.T) {
	body := `name: test
on: push
jobs:
  j:
    runs-on: ubuntu-latest
    steps:
      - uses: "actions/checkout@v4"
`
	p := writeTempFile(t, body)

	fake := &fakeResolver{
		resolve: map[string]string{
			"actions/checkout@v4": sha40,
		},
	}
	r, err := Plan([]string{p}, ModePin, fake, 4)
	if err != nil {
		t.Fatal(err)
	}
	for _, fp := range r.Files {
		Apply(fp)
		if err := fp.File.Write(); err != nil {
			t.Fatal(err)
		}
	}

	got, _ := os.ReadFile(p)
	want := `      - uses: "actions/checkout@` + sha40 + `" # v4`
	if !strings.Contains(string(got), want) {
		t.Errorf("quoted uses value was not preserved, got:\n%s", got)
	}
}

func TestApply_PinPreservesExistingComment(t *testing.T) {
	body := `name: test
on: push
jobs:
  j:
    runs-on: ubuntu-latest
    steps:
      - uses: foo/bar@v1 # renovate: pinDigest
`
	p := writeTempFile(t, body)

	fake := &fakeResolver{
		resolve: map[string]string{
			"foo/bar@v1": sha40,
		},
	}
	r, err := Plan([]string{p}, ModePin, fake, 4)
	if err != nil {
		t.Fatal(err)
	}
	for _, fp := range r.Files {
		Apply(fp)
		if err := fp.File.Write(); err != nil {
			t.Fatal(err)
		}
	}

	got, _ := os.ReadFile(p)
	want := `      - uses: foo/bar@` + sha40 + ` # v1 renovate: pinDigest`
	if !strings.Contains(string(got), want) {
		t.Errorf("existing comment was not preserved, got:\n%s", got)
	}
}
