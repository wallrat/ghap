package cmd

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"
)

func TestResolveAuthTokenPrefersFlagOverEnvAndGH(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "env-token")
	withAuthTestState(t, "flag-token", func(context.Context) ([]byte, error) {
		return []byte("gh-token\n"), nil
	})

	auth := resolveAuth()
	if auth.token != "flag-token" {
		t.Fatalf("resolveAuth().token = %q, want flag-token", auth.token)
	}
	if auth.source != authSourceFlag {
		t.Fatalf("resolveAuth().source = %q, want %q", auth.source, authSourceFlag)
	}
}

func TestResolveAuthTokenPrefersEnvOverGH(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "env-token")
	withAuthTestState(t, "", func(context.Context) ([]byte, error) {
		return []byte("gh-token\n"), nil
	})

	auth := resolveAuth()
	if auth.token != "env-token" {
		t.Fatalf("resolveAuth().token = %q, want env-token", auth.token)
	}
	if auth.source != authSourceEnv {
		t.Fatalf("resolveAuth().source = %q, want %q", auth.source, authSourceEnv)
	}
}

func TestResolveAuthTokenUsesGHWhenFlagAndEnvUnset(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	withAuthTestState(t, "", func(context.Context) ([]byte, error) {
		return []byte("gh-token\n"), nil
	})

	auth := resolveAuth()
	if auth.token != "gh-token" {
		t.Fatalf("resolveAuth().token = %q, want gh-token", auth.token)
	}
	if auth.source != authSourceGitHubCLI {
		t.Fatalf("resolveAuth().source = %q, want %q", auth.source, authSourceGitHubCLI)
	}
}

func TestGHAuthTokenFromCLITrimsWhitespace(t *testing.T) {
	withAuthTestState(t, "", func(context.Context) ([]byte, error) {
		return []byte("  gh-token\n"), nil
	})

	if got := ghAuthTokenFromCLI(time.Second); got != "gh-token" {
		t.Fatalf("ghAuthTokenFromCLI() = %q, want gh-token", got)
	}
}

func TestResolveAuthTokenFallsBackToAnonymousWhenGHFails(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	withAuthTestState(t, "", func(context.Context) ([]byte, error) {
		return nil, errors.New("gh unavailable")
	})

	auth := resolveAuth()
	if auth.token != "" {
		t.Fatalf("resolveAuth().token = %q, want anonymous", auth.token)
	}
	if auth.source != authSourceAnonymous {
		t.Fatalf("resolveAuth().source = %q, want %q", auth.source, authSourceAnonymous)
	}
}

func TestResolveAuthTokenFallsBackToAnonymousWhenGHReturnsEmpty(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	withAuthTestState(t, "", func(context.Context) ([]byte, error) {
		return []byte("\n"), nil
	})

	auth := resolveAuth()
	if auth.token != "" {
		t.Fatalf("resolveAuth().token = %q, want anonymous", auth.token)
	}
	if auth.source != authSourceAnonymous {
		t.Fatalf("resolveAuth().source = %q, want %q", auth.source, authSourceAnonymous)
	}
}

func TestResolveAuthTokenReturnsOnlyToken(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "env-token")
	withAuthTestState(t, "", func(context.Context) ([]byte, error) {
		return []byte("gh-token\n"), nil
	})

	if got := resolveAuthToken(); got != "env-token" {
		t.Fatalf("resolveAuthToken() = %q, want env-token", got)
	}
}

func TestWriteAuthSource(t *testing.T) {
	tests := []struct {
		name   string
		source authSource
		want   string
	}{
		{name: "flag", source: authSourceFlag, want: "auth: using --token\n"},
		{name: "env", source: authSourceEnv, want: "auth: using GITHUB_TOKEN\n"},
		{name: "gh", source: authSourceGitHubCLI, want: "auth: using GitHub CLI (gh auth token)\n"},
		{name: "anonymous", source: authSourceAnonymous, want: "auth: anonymous (60/hr cap)\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			writeAuthSource(&out, tt.source)
			if got := out.String(); got != tt.want {
				t.Fatalf("writeAuthSource() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGHAuthTokenFromCLIUsesTimeoutContext(t *testing.T) {
	withAuthTestState(t, "", func(ctx context.Context) ([]byte, error) {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("gh auth token context has no deadline")
		}
		return nil, errors.New("stop after deadline check")
	})

	_ = ghAuthTokenFromCLI(time.Second)
}

func withAuthTestState(t *testing.T, flagToken string, run func(context.Context) ([]byte, error)) {
	t.Helper()

	oldToken := g.token
	oldRun := runGHAuthToken
	g.token = flagToken
	runGHAuthToken = run
	t.Cleanup(func() {
		g.token = oldToken
		runGHAuthToken = oldRun
	})
}
