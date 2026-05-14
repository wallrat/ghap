package pinner

import (
	"sort"

	"github.com/wallrat/ghap/internal/workflow"
)

// Apply rewrites the in-memory file lines for every Change in fp. Caller is
// responsible for persisting via fp.File.Write() afterwards.
//
// Multiple matches on a single line are applied right-to-left to keep byte
// offsets valid.
func Apply(fp *FilePlan) {
	if len(fp.Changes) == 0 {
		return
	}
	byLine := map[int][]Change{}
	for _, c := range fp.Changes {
		byLine[c.LineIndex] = append(byLine[c.LineIndex], c)
	}
	for _, group := range byLine {
		sort.Slice(group, func(i, j int) bool {
			return group[i].match.Start > group[j].match.Start
		})
		for _, c := range group {
			repl := workflow.RenderUses(c.Action, c.AfterRef, c.AfterSrcRef, c.AfterTail, c.match.Quote)
			fp.File.ReplaceMatch(c.match, repl)
		}
	}
}
