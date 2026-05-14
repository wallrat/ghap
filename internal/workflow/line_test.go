package workflow

import "testing"

func TestFindUses(t *testing.T) {
	cases := []struct {
		name       string
		line       string
		wantOK     bool
		wantAction string
		wantRef    string
		wantSrcRef string
		wantTail   string
		wantIsSHA  bool
	}{
		{name: "tag", line: "      - uses: actions/checkout@v3", wantOK: true, wantAction: "actions/checkout", wantRef: "v3"},
		{name: "branch", line: "      - uses: actions/checkout@main", wantOK: true, wantAction: "actions/checkout", wantRef: "main"},
		{name: "double-quoted", line: `      - uses: "actions/checkout@v4"`, wantOK: true, wantAction: "actions/checkout", wantRef: "v4"},
		{name: "single-quoted", line: "      - uses: 'actions/checkout@v4'", wantOK: true, wantAction: "actions/checkout", wantRef: "v4"},
		{name: "quoted+comment", line: `      - uses: "actions/checkout@v4" # renovate: pinDigest`, wantOK: true, wantAction: "actions/checkout", wantRef: "v4", wantSrcRef: "renovate:", wantTail: " pinDigest"},
		{name: "sha", line: "      - uses: actions/checkout@b4ffde3b8c7e7e3b6b7e3e1e3b6b7e3e1e3b6b7e", wantOK: true, wantAction: "actions/checkout", wantRef: "b4ffde3b8c7e7e3b6b7e3e1e3b6b7e3e1e3b6b7e", wantIsSHA: true},
		{name: "sha+comment", line: "      - uses: actions/checkout@b4ffde3b8c7e7e3b6b7e3e1e3b6b7e3e1e3b6b7e # v3", wantOK: true, wantAction: "actions/checkout", wantRef: "b4ffde3b8c7e7e3b6b7e3e1e3b6b7e3e1e3b6b7e", wantSrcRef: "v3", wantIsSHA: true},
		{name: "sha+comment+tail", line: "      - uses: actions/checkout@b4ffde3b8c7e7e3b6b7e3e1e3b6b7e3e1e3b6b7e # v3 (manually verified)", wantOK: true, wantAction: "actions/checkout", wantRef: "b4ffde3b8c7e7e3b6b7e3e1e3b6b7e3e1e3b6b7e", wantSrcRef: "v3", wantTail: " (manually verified)", wantIsSHA: true},
		{name: "subpath", line: "      - uses: actions/cache/save@v3", wantOK: true, wantAction: "actions/cache/save", wantRef: "v3"},
		{name: "name-then-uses", line: "        uses: codecov/codecov-action@v3.1.4", wantOK: true, wantAction: "codecov/codecov-action", wantRef: "v3.1.4"},
		{name: "docker", line: "      - uses: docker://alpine:3.18", wantOK: false},
		{name: "local-dot-slash", line: "      - uses: ./my-action", wantOK: false},
		{name: "local-dot-dot", line: "      - uses: ../shared/action", wantOK: false},
		{name: "no-uses", line: "      - run: echo hi", wantOK: false},
		{name: "run-contains-uses", line: `      - run: echo "uses: actions/checkout@v3"`, wantOK: false},
		{name: "comment-contains-uses", line: "      # uses: actions/checkout@v3", wantOK: false},
		{name: "name-contains-uses", line: "      - name: uses: actions/checkout@v3", wantOK: false},
		{name: "missing-at", line: "      - uses: actions/checkout", wantOK: false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m, ok := FindUses(c.line, 0)
			if ok != c.wantOK {
				t.Fatalf("ok=%v want %v", ok, c.wantOK)
			}
			if !ok {
				return
			}
			if got := m.Action.Action(); got != c.wantAction {
				t.Errorf("action=%q want %q", got, c.wantAction)
			}
			if m.Action.Ref != c.wantRef {
				t.Errorf("ref=%q want %q", m.Action.Ref, c.wantRef)
			}
			if m.Action.SrcRef != c.wantSrcRef {
				t.Errorf("srcref=%q want %q", m.Action.SrcRef, c.wantSrcRef)
			}
			if m.Action.CommentTail != c.wantTail {
				t.Errorf("tail=%q want %q", m.Action.CommentTail, c.wantTail)
			}
			if m.Action.IsSHA != c.wantIsSHA {
				t.Errorf("isSHA=%v want %v", m.Action.IsSHA, c.wantIsSHA)
			}
		})
	}
}

func TestRenderUses(t *testing.T) {
	cases := []struct {
		name, action, ref, srcRef, tail, quote, want string
	}{
		{"pin", "actions/checkout", "abc123", "v3", "", "", "uses: actions/checkout@abc123 # v3"},
		{"pin-with-tail", "actions/checkout", "abc123", "v3", " (verified)", "", "uses: actions/checkout@abc123 # v3 (verified)"},
		{"latest-no-comment", "actions/checkout", "abc123", "", "", "", "uses: actions/checkout@abc123"},
		{"subpath", "actions/cache/save", "abc123", "v3", "", "", "uses: actions/cache/save@abc123 # v3"},
		{"quoted", "actions/checkout", "abc123", "v3", "", `"`, `uses: "actions/checkout@abc123" # v3`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := RenderUses(c.action, c.ref, c.srcRef, c.tail, c.quote)
			if got != c.want {
				t.Errorf("got %q want %q", got, c.want)
			}
		})
	}
}

func TestReplaceMatchRoundTrip(t *testing.T) {
	src := "      - uses: actions/checkout@v3 # earlier comment"
	m, ok := FindUses(src, 0)
	if !ok {
		t.Fatal("expected match")
	}
	f := &File{Lines: []string{src}}
	f.ReplaceMatch(m, "uses: actions/checkout@deadbeefdeadbeefdeadbeefdeadbeefdeadbeef # v3")
	want := "      - uses: actions/checkout@deadbeefdeadbeefdeadbeefdeadbeefdeadbeef # v3"
	if f.Lines[0] != want {
		t.Errorf("got %q\nwant %q", f.Lines[0], want)
	}
}
