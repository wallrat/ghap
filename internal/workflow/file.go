package workflow

import (
	"os"
	"strings"
)

// File is the contents of a workflow file, split into lines without their
// terminating newline (and with \r preserved for CRLF inputs).
type File struct {
	Path           string
	Lines          []string
	TrailingNewln  bool // whether the original file ended with a newline
	Mode           os.FileMode
}

// Read loads a workflow file into memory.
func Read(path string) (*File, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	s := string(data)
	trailing := strings.HasSuffix(s, "\n")
	if trailing {
		s = strings.TrimSuffix(s, "\n")
	}
	lines := strings.Split(s, "\n")
	return &File{Path: path, Lines: lines, TrailingNewln: trailing, Mode: info.Mode()}, nil
}

// Write persists the file using the original mode and newline convention.
func (f *File) Write() error {
	out := strings.Join(f.Lines, "\n")
	if f.TrailingNewln {
		out += "\n"
	}
	return os.WriteFile(f.Path, []byte(out), f.Mode.Perm())
}

// HasWorkflowMarker reports whether any line begins with `on:` or `jobs:` at
// the start of the line (after optional whitespace). Mirrors legacy.
func (f *File) HasWorkflowMarker() bool {
	for _, ln := range f.Lines {
		t := strings.TrimSpace(ln)
		if strings.HasPrefix(t, "on:") || strings.HasPrefix(t, "jobs:") {
			return true
		}
	}
	return false
}

// FindAllUses returns every `uses:` match across the file.
func (f *File) FindAllUses() []UsesMatch {
	var out []UsesMatch
	for i, ln := range f.Lines {
		if m, ok := FindUses(ln, i); ok {
			out = append(out, m)
		}
	}
	return out
}

// ReplaceMatch swaps the substring at m's span in the corresponding line with
// `repl`. Safe to call for one match at a time; if you have multiple matches
// on the same line, apply right-to-left to keep offsets valid.
func (f *File) ReplaceMatch(m UsesMatch, repl string) {
	ln := f.Lines[m.LineIndex]
	f.Lines[m.LineIndex] = ln[:m.Start] + repl + ln[m.End:]
}
