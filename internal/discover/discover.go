package discover

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wallra/ghap/internal/workflow"
)

// Files resolves the supplied paths to a deduped, sorted list of workflow
// files. Each path may be: a workflow file, a directory of workflows, or a
// git repo root containing `.github/workflows/`.
func Files(paths []string) ([]string, error) {
	seen := map[string]struct{}{}
	var out []string
	add := func(p string) {
		ap, err := filepath.Abs(p)
		if err != nil {
			ap = p
		}
		if _, ok := seen[ap]; ok {
			return
		}
		seen[ap] = struct{}{}
		out = append(out, p)
	}

	for _, p := range paths {
		st, err := os.Stat(p)
		if err != nil {
			return nil, err
		}
		if !st.IsDir() {
			if isYAML(p) && isWorkflowFile(p) {
				add(p)
			}
			continue
		}
		// Directory: if it has a .github/workflows subdir, prefer that; else
		// walk recursively.
		ghw := filepath.Join(p, ".github", "workflows")
		if st2, err := os.Stat(ghw); err == nil && st2.IsDir() {
			if err := walkDir(ghw, add); err != nil {
				return nil, err
			}
			continue
		}
		if err := walkDir(p, add); err != nil {
			return nil, err
		}
	}
	sort.Strings(out)
	return out, nil
}

func walkDir(root string, add func(string)) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		if isYAML(path) && isWorkflowFile(path) {
			add(path)
		}
		return nil
	})
}

func isYAML(p string) bool {
	l := strings.ToLower(p)
	return strings.HasSuffix(l, ".yml") || strings.HasSuffix(l, ".yaml")
}

// isWorkflowFile uses the same heuristic as the legacy tool: the file must
// contain a top-level `on:` or `jobs:` key. We reuse workflow.Read so we get
// the same line splitting.
func isWorkflowFile(p string) bool {
	f, err := workflow.Read(p)
	if err != nil {
		return false
	}
	return f.HasWorkflowMarker()
}
