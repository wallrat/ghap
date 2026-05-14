package resolver

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/go-github/v72/github"
)

func TestResolveRefPrefersTagOverBranch(t *testing.T) {
	const tagSHA = "1111111111111111111111111111111111111111"
	const branchSHA = "2222222222222222222222222222222222222222"

	var branchLookups int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo/git/ref/tags/v1":
			fmt.Fprintf(w, `{"ref":"refs/tags/v1","object":{"type":"commit","sha":%q}}`, tagSHA)
		case "/repos/owner/repo/git/ref/heads/v1":
			branchLookups++
			fmt.Fprintf(w, `{"ref":"refs/heads/v1","object":{"type":"commit","sha":%q}}`, branchSHA)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	baseURL, err := url.Parse(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	r := &Resolver{GH: github.NewClient(srv.Client())}
	r.GH.BaseURL = baseURL

	got, err := r.ResolveRef("owner", "repo", "v1")
	if err != nil {
		t.Fatal(err)
	}
	if got != tagSHA {
		t.Fatalf("ResolveRef returned %q, want tag SHA %q", got, tagSHA)
	}
	if branchLookups != 0 {
		t.Fatalf("ResolveRef looked up branch despite tag match")
	}
}

func TestResolveRefReturnsTagLookupErrorBeforeBranch(t *testing.T) {
	const branchSHA = "2222222222222222222222222222222222222222"

	var branchLookups int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo/git/ref/tags/v1":
			http.Error(w, "upstream unavailable", http.StatusInternalServerError)
		case "/repos/owner/repo/git/ref/heads/v1":
			branchLookups++
			fmt.Fprintf(w, `{"ref":"refs/heads/v1","object":{"type":"commit","sha":%q}}`, branchSHA)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	r := testResolver(t, srv)

	got, err := r.ResolveRef("owner", "repo", "v1")
	if err == nil {
		t.Fatalf("ResolveRef returned nil error and SHA %q, want tag lookup error", got)
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("ResolveRef error = %q, want status 500", err.Error())
	}
	if branchLookups != 0 {
		t.Fatalf("ResolveRef looked up branch after non-404 tag error")
	}
}

func TestResolveRefFallsBackToBranchWhenTagNotFound(t *testing.T) {
	const branchSHA = "2222222222222222222222222222222222222222"

	var branchLookups int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo/git/ref/tags/v1":
			http.NotFound(w, r)
		case "/repos/owner/repo/git/ref/heads/v1":
			branchLookups++
			fmt.Fprintf(w, `{"ref":"refs/heads/v1","object":{"type":"commit","sha":%q}}`, branchSHA)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	r := testResolver(t, srv)

	got, err := r.ResolveRef("owner", "repo", "v1")
	if err != nil {
		t.Fatal(err)
	}
	if got != branchSHA {
		t.Fatalf("ResolveRef returned %q, want branch SHA %q", got, branchSHA)
	}
	if branchLookups != 1 {
		t.Fatalf("ResolveRef branch lookups = %d, want 1", branchLookups)
	}
}

func TestResolveRefReturnsBranchLookupError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo/git/ref/tags/v1":
			http.NotFound(w, r)
		case "/repos/owner/repo/git/ref/heads/v1":
			http.Error(w, "upstream unavailable", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	r := testResolver(t, srv)

	got, err := r.ResolveRef("owner", "repo", "v1")
	if err == nil {
		t.Fatalf("ResolveRef returned nil error and SHA %q, want branch lookup error", got)
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("ResolveRef error = %q, want status 500", err.Error())
	}
	if strings.Contains(err.Error(), "not a branch or tag") {
		t.Fatalf("ResolveRef masked branch lookup error as %q", err.Error())
	}
}

func testResolver(t *testing.T, srv *httptest.Server) *Resolver {
	t.Helper()

	baseURL, err := url.Parse(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	r := &Resolver{GH: github.NewClient(srv.Client())}
	r.GH.BaseURL = baseURL
	return r
}
