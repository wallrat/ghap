package pinner

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var updateGoldens = flag.Bool("update", false, "rewrite e2e golden files")

// preUpload mirrors line 4's pre-existing SHA — resolveRef returns it so
// update sees an unchanged pin and skips the line. Resolver keys are
// owner/repo (no subpath), so the cache/restore subpath line reuses
// actions/cache's SHA on purpose.
const (
	preUpload    = "2222222222222222222222222222222222222222"
	resCheckout  = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	resCPR       = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	resCache     = "cccccccccccccccccccccccccccccccccccccccc"
	resSetupNode = "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	latCheckout  = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa5"
	latCPR       = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb7"
	latCache     = "ccccccccccccccccccccccccccccccccccccccc4"
	latUpload    = "2222222222222222222222222222222222222225"
	latSetupGo   = "3333333333333333333333333333333333333335"
	latSetupNode = "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee5"
)

func e2eFakeResolver() *fakeResolver {
	return &fakeResolver{
		resolve: map[string]string{
			"actions/checkout@v3":                  resCheckout,
			"peter-evans/create-pull-request@main": resCPR,
			"actions/cache@v3":                     resCache, // also serves cache/restore@v3
			"actions/upload-artifact@v3":           preUpload, // same → unchanged skip
			"actions/setup-node@v3":                resSetupNode,
		},
		latest: map[string]string{
			"actions/checkout":                latCheckout,
			"peter-evans/create-pull-request": latCPR,
			"actions/cache":                   latCache, // also serves cache/restore
			"actions/upload-artifact":         latUpload,
			"actions/setup-go":                latSetupGo,
			"actions/setup-node":              latSetupNode,
		},
		latestTag: map[string]string{
			"actions/checkout":                "v5",
			"peter-evans/create-pull-request": "v7",
			"actions/cache":                   "v4",
			"actions/upload-artifact":         "v4",
			"actions/setup-go":                "v5",
			"actions/setup-node":              "v5",
		},
	}
}

func runE2E(t *testing.T, mode Mode, goldenName string) {
	t.Helper()
	src, err := os.ReadFile("testdata/workflow.yml")
	if err != nil {
		t.Fatal(err)
	}
	tmpPath := writeTempFile(t, string(src))

	r, err := Plan([]string{tmpPath}, mode, e2eFakeResolver(), 4)
	if err != nil {
		t.Fatal(err)
	}
	for _, fp := range r.Files {
		Apply(fp)
		if err := fp.File.Write(); err != nil {
			t.Fatal(err)
		}
	}

	got, err := os.ReadFile(tmpPath)
	if err != nil {
		t.Fatal(err)
	}
	goldenPath := filepath.Join("testdata", goldenName)
	if *updateGoldens {
		if err := os.WriteFile(goldenPath, got, 0644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden: %v (run `go test ./internal/pinner -run E2E -update` to generate)", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("output mismatch for %s\n--- want ---\n%s\n--- got ---\n%s", goldenName, want, got)
	}
}

func TestE2E_Pin(t *testing.T) {
	runE2E(t, ModePin, "workflow.pin.golden.yml")
}

func TestE2E_Update(t *testing.T) {
	runE2E(t, ModeUpdate, "workflow.update.golden.yml")
}

func TestE2E_UpdateLatest(t *testing.T) {
	runE2E(t, ModeLatest, "workflow.latest.golden.yml")
}
