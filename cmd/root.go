package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/wallra/ghap/internal/discover"
	"github.com/wallra/ghap/internal/pinner"
	"github.com/wallra/ghap/internal/resolver"
)

// Version is set at link time.
var Version = "dev"

type globals struct {
	token       string
	concurrency int
	dryRun      bool
	verbose     bool
	interactive bool
}

var g globals

var rootCmd = &cobra.Command{
	Use:   "ghap [paths...]",
	Short: "Pin and update GitHub Actions to specific commit SHAs",
	Long: `ghap (GitHub Action Pinner) keeps GitHub Actions references in workflow
YAMLs locked to immutable commit SHAs, then keeps them up to date.

Commands:
  (no command)   Print a table per workflow: current ref, what it'd pin to,
                 and the latest release SHA. Read-only.
  pin            Replace each unpinned 'uses: owner/repo@<ref>' with the
                 resolved SHA and a '# <ref>' source comment.
  update         Pin unpinned actions AND refresh pinned ones in one pass.
                 With --latest, bump every action to its latest release tag.

Each command accepts files, directories, or git-repo roots (containing
.github/workflows/). Authentication uses --token, $GITHUB_TOKEN, or GitHub
CLI auth; otherwise ghap runs anonymously against GitHub's 60/hr cap.`,
	Example: `  ghap .github/workflows                    # inspect every workflow in the repo
  ghap pin .github/workflows/ci.yml         # pin one file
  ghap update --latest .                    # bump every action to latest tag
  ghap update -i .                          # interactively confirm each change
  ghap --dry-run update .                   # preview without writing`,
	Version:       Version,
	SilenceUsage:  true,
	SilenceErrors: false,
	Args:          cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Usage()
		}
		return runTable(args)
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	pflags := rootCmd.PersistentFlags()
	pflags.StringVar(&g.token, "token", "", "GitHub token (defaults to $GITHUB_TOKEN, then GitHub CLI auth; anonymous if unset)")
	pflags.IntVar(&g.concurrency, "concurrency", 8, "max concurrent GitHub API requests")
	pflags.BoolVar(&g.dryRun, "dry-run", false, "do not write files (applies to pin/update)")
	pflags.BoolVarP(&g.verbose, "verbose", "v", false, "verbose output (include skipped items)")

	rootCmd.AddCommand(pinCmd)
	rootCmd.AddCommand(updateCmd)
}

const ghAuthTokenTimeout = 2 * time.Second

var runGHAuthToken = func(ctx context.Context) ([]byte, error) {
	return exec.CommandContext(ctx, "gh", "auth", "token").Output()
}

type authSource string

const (
	authSourceFlag      authSource = "--token"
	authSourceEnv       authSource = "GITHUB_TOKEN"
	authSourceGitHubCLI authSource = "GitHub CLI (gh auth token)"
	authSourceAnonymous authSource = "anonymous"
)

type authConfig struct {
	token  string
	source authSource
}

// resolveAuth honors --token, $GITHUB_TOKEN, then GitHub CLI auth.
// Empty token means anonymous API access.
func resolveAuth() authConfig {
	if g.token != "" {
		return authConfig{token: g.token, source: authSourceFlag}
	}
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		return authConfig{token: token, source: authSourceEnv}
	}
	if token := ghAuthTokenFromCLI(ghAuthTokenTimeout); token != "" {
		return authConfig{token: token, source: authSourceGitHubCLI}
	}
	return authConfig{source: authSourceAnonymous}
}

func resolveAuthToken() string {
	return resolveAuth().token
}

func ghAuthTokenFromCLI(timeout time.Duration) string {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	out, err := runGHAuthToken(ctx)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func newResolver() (*resolver.Resolver, authSource) {
	auth := resolveAuth()
	return resolver.New(auth.token), auth.source
}

func reportAuthSource(source authSource) {
	writeAuthSource(os.Stderr, source)
}

func writeAuthSource(w io.Writer, source authSource) {
	switch source {
	case authSourceFlag:
		fmt.Fprintln(w, "auth: using --token")
	case authSourceEnv:
		fmt.Fprintln(w, "auth: using GITHUB_TOKEN")
	case authSourceGitHubCLI:
		fmt.Fprintln(w, "auth: using GitHub CLI (gh auth token)")
	default:
		fmt.Fprintln(w, "auth: anonymous (60/hr cap)")
	}
}

func runTable(args []string) error {
	paths, err := discover.Files(args)
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		fmt.Fprintln(os.Stderr, "no workflow files found")
		return nil
	}
	res, authSource := newResolver()
	if g.verbose {
		reportAuthSource(authSource)
	}
	rows, err := pinner.BuildTableRows(paths, res, g.concurrency)
	if err != nil {
		return err
	}
	pinner.RenderTable(os.Stdout, rows)
	if g.verbose {
		reportRowErrors(rows)
	}
	return nil
}

// reportRowErrors emits per-row resolver errors to stderr. Rate-limit errors
// are coalesced into one line at the top (with affected count). Other errors
// are deduped by full message and listed with their affected rows.
func reportRowErrors(rows []pinner.TableRow) {
	rateLimited := 0
	authed := false
	type bucket struct {
		err  string
		rows []string
	}
	idx := map[string]*bucket{}
	order := []string{}
	add := func(col string, err error, row pinner.TableRow) {
		if err == nil {
			return
		}
		var rl *resolver.RateLimitedError
		if errors.As(err, &rl) {
			rateLimited++
			authed = rl.Authenticated
			return
		}
		key := col + "|" + err.Error()
		b, ok := idx[key]
		if !ok {
			b = &bucket{err: col + " column error: " + err.Error()}
			idx[key] = b
			order = append(order, key)
		}
		b.rows = append(b.rows, fmt.Sprintf("%s:%d %s", row.Workflow, row.LineIndex+1, row.Action))
	}
	for _, r := range rows {
		add("Pin", r.PinErr, r)
		add("Latest", r.LatestErr, r)
	}
	if rateLimited > 0 {
		cap := "anonymous (60/hr)"
		fix := "set GITHUB_TOKEN, pass --token, or run gh auth login to bump to 5000/hr"
		if authed {
			cap = "authenticated (5000/hr)"
			fix = "wait for reset"
		}
		fmt.Fprintf(os.Stderr, "github rate limit hit on %s cap (%d column lookups skipped) — %s\n", cap, rateLimited, fix)
	}
	for _, k := range order {
		b := idx[k]
		fmt.Fprintln(os.Stderr, b.err)
		for _, r := range b.rows {
			fmt.Fprintf(os.Stderr, "  - %s\n", r)
		}
	}
}
