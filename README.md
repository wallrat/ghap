# ghap — GitHub Action Pinner

[![Go Reference](https://pkg.go.dev/badge/github.com/wallrat/ghap.svg)](https://pkg.go.dev/github.com/wallrat/ghap)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

A small Go CLI that locks GitHub Actions in your workflow YAMLs to immutable commit SHAs — and keeps them up to date.

```yaml
# before
- uses: actions/checkout@v4

# after `ghap pin`
- uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # v4
```

## Why

Tags and branches in `uses:` are mutable. A malicious or compromised maintainer can re-point a tag at hostile code, and your next CI run executes it — silently, with your secrets. GitHub's own [security hardening guide](https://docs.github.com/en/actions/security-guides/security-hardening-for-github-actions#using-third-party-actions) recommends pinning to full commit SHAs.

Pinning by hand is tedious; un-pinning to update is worse. `ghap` does both:

- **Pin** every action to a full SHA, with the original tag/branch preserved as a trailing comment.
- **Update** later — `ghap` re-resolves the comment back to the *current* SHA for that tag, or bumps to the latest release.
- Works on a single file, a folder of workflows, or a repo root (auto-discovers `.github/workflows/`).
- Concurrent GitHub API calls with in-process HTTP caching and request deduping; honors `$GITHUB_TOKEN` and GitHub CLI auth.
- Read-only inspection mode prints a table of *current → pin → latest* for every action.
- Interactive (`-i`) and `--dry-run` modes for safe rollout.

## Install

### Pre-built binaries (recommended)

Grab a tarball/zip (or `.deb`, `.rpm`, `.apk`) for your platform from the [latest GitHub release](https://github.com/wallrat/ghap/releases/latest).

### One-off (no install)

Requires Go 1.25+.

```sh
go run github.com/wallrat/ghap@latest .
```

### From source

Requires Go 1.25+.

```sh
go install github.com/wallrat/ghap@latest
```

Or:

```sh
git clone https://github.com/wallrat/ghap.git
cd ghap
go build -o ghap .
```

## Authentication

`ghap` calls the GitHub API to resolve refs and tags. Anonymous requests are capped at 60/hr — enough for a quick look, not enough for a real repo. Export a token, pass one directly, or authenticate the GitHub CLI to lift the cap to 5000/hr:

```sh
export GITHUB_TOKEN=ghp_xxx
# or
ghap --token ghp_xxx ...
# or
gh auth login
```

A token with no scopes is sufficient for public repos.

## Usage

### Inspect (no command)

Print a table showing each action's current ref, what it would pin to, and the latest release SHA. Read-only.

```sh
ghap .github/workflows
ghap .                              # repo root, auto-discovers .github/workflows/
```

### `pin`

Replace each unpinned `uses: owner/repo@<ref>` with the resolved SHA, annotating the original ref as a trailing comment. Already-pinned lines are left alone.

```sh
ghap pin .github/workflows/ci.yml
ghap pin .
```

### `update`

In one pass:

- **Unpinned** lines are pinned (same as `pin`).
- **Pinned with `# <ref>` comment** are re-resolved against `<ref>` — the line moves if the tag/branch has advanced.
- **Pinned without a comment** are bumped to the latest release tag.
- With `--latest`, *every* action is bumped to its latest release tag and re-annotated, dropping any prior `# <ref>` comment.

```sh
ghap update .
ghap update --latest .
```

### Common flags

| Flag | Description |
| --- | --- |
| `-i`, `--interactive` | Prompt y/n for each change (local to `pin` and `update`). |
| `--dry-run` | Plan only — print the diff table, don't write files. |
| `-v`, `--verbose` | Include skipped lines and resolver errors. |
| `--token` | GitHub token (overrides `$GITHUB_TOKEN` and GitHub CLI auth). |
| `--concurrency` | Max concurrent API requests (default 8). |

### Examples

```sh
ghap .github/workflows                    # inspect every workflow in the repo
ghap pin .github/workflows/ci.yml         # pin one file
ghap update --latest .                    # bump every action to latest release
ghap update -i .                          # interactively confirm each change
ghap --dry-run update .                   # preview without writing
```
#### Inspect

```
❯ ghap .
.github/workflows/release.yml
┌────────────────────────────┬─────────────────────┬────────────┬─────────────────────┐
│Action                      │Current              │Pin         │Latest               │
├────────────────────────────┼─────────────────────┼────────────┼─────────────────────┤
│actions/checkout            │de0fac2e4500 (v6.0.2)│de0fac2e4500│de0fac2e4500 (v6.0.2)│
│actions/setup-go            │4a3601121dd0 (v6.4.0)│4a3601121dd0│4a3601121dd0 (v6.4.0)│
│goreleaser/goreleaser-action│1a80836c5c9d (v7.2.1)│1a80836c5c9d│1a80836c5c9d (v7.2.1)│
└────────────────────────────┴─────────────────────┴────────────┴─────────────────────┘
```


## How it works

Each workflow file is scanned line-by-line for `uses: owner/repo@<ref>` and any trailing `# <comment>`. `ghap` only rewrites the ref portion of the line — surrounding whitespace, formatting, and any non-source comments (e.g. `# v3 (verified)`) are preserved byte-for-byte. SHAs come from the GitHub REST API (`go-github`), cached in-process via `httpcache` (memory transport) and deduped with `singleflight`, so a workflow with 6× `actions/checkout@v3` makes one API call per run.

## Contributing

I'm not accepting contributions right now. Open an issue if found you found a bug.

## License

MIT — see [LICENSE](LICENSE).
