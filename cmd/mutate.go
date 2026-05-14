package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"

	"github.com/charmbracelet/x/term"
	"github.com/wallra/ghap/internal/discover"
	"github.com/wallra/ghap/internal/pinner"
)

type mutateMode int

const (
	modePin mutateMode = iota
	modeUpdate
	modeLatest
)

func (m mutateMode) toPinnerMode() pinner.Mode {
	switch m {
	case modePin:
		return pinner.ModePin
	case modeUpdate:
		return pinner.ModeUpdate
	case modeLatest:
		return pinner.ModeLatest
	}
	return pinner.ModePin
}

func runMutate(args []string, mode mutateMode, interactive bool) error {
	paths, err := discover.Files(args)
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		fmt.Fprintln(os.Stderr, "no workflow files found")
		return nil
	}
	res := newResolver()
	result, err := pinner.Plan(paths, mode.toPinnerMode(), res, g.concurrency)
	if err != nil {
		return err
	}

	// Filter changes interactively if requested.
	if interactive {
		readKey, err := newPromptKeyReader(os.Stdin)
		if err != nil {
			return err
		}
		for _, fp := range result.Files {
			kept := fp.Changes[:0]
			for _, c := range fp.Changes {
				accept, err := promptApply(readKey, os.Stderr, c)
				if err != nil {
					return err
				}
				if accept {
					kept = append(kept, c)
				}
			}
			fp.Changes = kept
		}
	}

	for _, fp := range result.Files {
		if g.verbose {
			for _, c := range fp.Changes {
				pinner.PrintChange(os.Stderr, c)
			}
			for _, s := range fp.Skipped {
				fmt.Fprintf(os.Stderr, "skip   file=%s line=%d action=%s ref=%s reason=%s",
					s.File, s.LineIndex+1, s.Action, s.Ref, s.Reason)
				if s.Err != nil {
					fmt.Fprintf(os.Stderr, " err=%v", s.Err)
				}
				fmt.Fprintln(os.Stderr)
			}
		}
		if len(fp.Changes) == 0 || g.dryRun {
			continue
		}
		pinner.Apply(fp)
		if err := fp.File.Write(); err != nil {
			return fmt.Errorf("write %s: %w", fp.File.Path, err)
		}
	}

	// Summary table: one row per applied (or planned, in dry-run) change.
	rows, err := pinner.BuildChangeRows(result, res, g.concurrency)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		fmt.Fprintln(os.Stderr, "no changes")
	} else {
		fmt.Fprintln(os.Stdout)
		pinner.RenderChangeTable(os.Stdout, rows)
	}
	if g.dryRun {
		fmt.Fprintln(os.Stderr, "(dry-run: no files written)")
	}
	return nil
}

type promptKeyReader func() (byte, error)

func newPromptKeyReader(f *os.File) (promptKeyReader, error) {
	reader := bufio.NewReader(f)
	if !term.IsTerminal(f.Fd()) {
		return reader.ReadByte, nil
	}
	return func() (byte, error) {
		state, err := term.MakeRaw(f.Fd())
		if err != nil {
			return 0, err
		}
		key, readErr := reader.ReadByte()
		restoreErr := term.Restore(f.Fd(), state)
		if readErr != nil {
			return 0, readErr
		}
		if restoreErr != nil {
			return 0, restoreErr
		}
		return key, nil
	}, nil
}

func promptApply(readKey promptKeyReader, w io.Writer, c pinner.Change) (bool, error) {
	prompt := fmt.Sprintf("%s:%d %s %s -> %s Apply? y/n ",
		c.File, c.LineIndex+1, c.Action, c.BeforeRef, c.AfterRef)
	for {
		fmt.Fprint(w, prompt)
		key, err := readKey()
		if err != nil {
			return false, err
		}
		switch key {
		case 'y', 'Y':
			fmt.Fprintln(w, "y")
			return true, nil
		case 'n', 'N', '\r', '\n':
			fmt.Fprintln(w, "n")
			return false, nil
		default:
			fmt.Fprintln(w, string(key))
			fmt.Fprintln(w, "Please answer y, n, or enter.")
		}
	}
}
