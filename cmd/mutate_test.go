package cmd

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/wallrat/ghap/internal/pinner"
)

func TestPromptApplyAcceptsY(t *testing.T) {
	var out bytes.Buffer
	change := pinner.Change{
		File:      ".github/workflows/ci.yml",
		LineIndex: 12,
		Action:    "actions/checkout",
		BeforeRef: "v4",
		AfterRef:  "1234567890abcdef",
	}

	accepted, err := promptApply(testPromptKeyReader("y"), &out, change)
	if err != nil {
		t.Fatalf("promptApply returned error: %v", err)
	}
	if !accepted {
		t.Fatal("promptApply returned accepted=false")
	}

	want := ".github/workflows/ci.yml:13 actions/checkout v4 -> 1234567890abcdef Apply? y/n y\n"
	if got := out.String(); got != want {
		t.Fatalf("prompt output mismatch\ngot:  %q\nwant: %q", got, want)
	}
}

func TestPromptApplyRejectsN(t *testing.T) {
	var out bytes.Buffer
	change := pinner.Change{
		File:      ".github/workflows/ci.yml",
		LineIndex: 0,
		Action:    "docker/login-action",
		BeforeRef: "main",
		AfterRef:  "abcdef",
	}

	accepted, err := promptApply(testPromptKeyReader("n"), &out, change)
	if err != nil {
		t.Fatalf("promptApply returned error: %v", err)
	}
	if accepted {
		t.Fatal("promptApply returned accepted=true")
	}

	want := ".github/workflows/ci.yml:1 docker/login-action main -> abcdef Apply? y/n n\n"
	if got := out.String(); got != want {
		t.Fatalf("prompt output mismatch\ngot:  %q\nwant: %q", got, want)
	}
}

func TestPromptApplyDefaultsEnterToNo(t *testing.T) {
	var out bytes.Buffer
	change := pinner.Change{
		File:      ".github/workflows/ci.yml",
		LineIndex: 0,
		Action:    "docker/login-action",
		BeforeRef: "main",
		AfterRef:  "abcdef",
	}

	accepted, err := promptApply(testPromptKeyReader("\n"), &out, change)
	if err != nil {
		t.Fatalf("promptApply returned error: %v", err)
	}
	if accepted {
		t.Fatal("promptApply returned accepted=true")
	}

	want := ".github/workflows/ci.yml:1 docker/login-action main -> abcdef Apply? y/n n\n"
	if got := out.String(); got != want {
		t.Fatalf("prompt output mismatch\ngot:  %q\nwant: %q", got, want)
	}
}

func TestPromptApplyRetriesInvalidAnswer(t *testing.T) {
	var out bytes.Buffer
	change := pinner.Change{
		File:      "ci.yml",
		LineIndex: 1,
		Action:    "owner/repo",
		BeforeRef: "v1",
		AfterRef:  "v2",
	}

	accepted, err := promptApply(testPromptKeyReader("?y"), &out, change)
	if err != nil {
		t.Fatalf("promptApply returned error: %v", err)
	}
	if !accepted {
		t.Fatal("promptApply returned accepted=false")
	}

	want := "ci.yml:2 owner/repo v1 -> v2 Apply? y/n ?\nPlease answer y, n, or enter.\nci.yml:2 owner/repo v1 -> v2 Apply? y/n y\n"
	if got := out.String(); got != want {
		t.Fatalf("prompt output mismatch\ngot:  %q\nwant: %q", got, want)
	}
}

func testPromptKeyReader(input string) promptKeyReader {
	r := strings.NewReader(input)
	return func() (byte, error) {
		b, err := r.ReadByte()
		if err == io.EOF {
			return 0, err
		}
		return b, err
	}
}
