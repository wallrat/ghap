package discover

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const wfBody = "name: t\non: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps: []\n"
const nonWfBody = "foo: bar\n"

func write(t *testing.T, p, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestFiles_DirectFile(t *testing.T) {
	d := t.TempDir()
	wf := filepath.Join(d, "a.yml")
	nf := filepath.Join(d, "b.yml")
	write(t, wf, wfBody)
	write(t, nf, nonWfBody)

	got, err := Files([]string{wf})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != wf {
		t.Errorf("got %v", got)
	}
	got, _ = Files([]string{nf})
	if len(got) != 0 {
		t.Errorf("expected non-workflow filtered, got %v", got)
	}
}

func TestFiles_RecursiveDir(t *testing.T) {
	d := t.TempDir()
	write(t, filepath.Join(d, "a.yml"), wfBody)
	write(t, filepath.Join(d, "nested", "b.yaml"), wfBody)
	write(t, filepath.Join(d, "nested", "c.txt"), wfBody)
	write(t, filepath.Join(d, "d.yml"), nonWfBody)

	got, err := Files([]string{d})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("got %v", got)
	}
}

func TestFiles_RepoRootPrefersDotGithub(t *testing.T) {
	d := t.TempDir()
	// Workflow at .github/workflows/* should be picked up. Other yml files
	// outside .github should be ignored when the repo has .github/workflows.
	write(t, filepath.Join(d, ".github", "workflows", "ci.yml"), wfBody)
	write(t, filepath.Join(d, "other.yml"), wfBody) // should NOT be picked up

	got, err := Files([]string{d})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !strings.Contains(got[0], filepath.Join(".github", "workflows")) {
		t.Errorf("expected only .github/workflows/ci.yml, got %v", got)
	}
}

func TestFiles_EmptyReturnsEmpty(t *testing.T) {
	got, err := Files(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no paths for empty input, got %v", got)
	}
}
