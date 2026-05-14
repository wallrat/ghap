package workflow

import (
	"regexp"

	"github.com/wallrat/ghap/internal/action"
)

// Captures:
//
//	1: YAML prefix (indentation and optional list marker)
//	2: optional opening quote
//	3: action ("owner/repo[/sub]")
//	4: ref
//	5: optional closing quote
//	6: optional comment first-token (source ref)
//	7: optional comment tail (everything after the first token incl. leading space)
var usesRegex = regexp.MustCompile(`^([ \t]*(?:-[ \t]*)?)uses:[ \t]*(['"]?)([^\s@'"]+)@([^\s#'"]+)(['"]?)(?:[ \t]+#[ \t]*(\S+)([^\r\n]*))?`)

// UsesMatch is one `uses:` occurrence within a single line.
type UsesMatch struct {
	LineIndex int        // 0-based
	Start     int        // byte offset in line where match begins
	End       int        // byte offset where match ends (exclusive)
	Quote     string     // optional quote style around the uses value: ' or "
	Action    action.Ref // parsed action ref
}

// FindUses scans a single line. Returns the match if the line has a pinnable
// `uses:` directive, ok=false otherwise.
func FindUses(line string, lineIdx int) (UsesMatch, bool) {
	idx := usesRegex.FindStringSubmatchIndex(line)
	if idx == nil {
		return UsesMatch{}, false
	}
	sub := func(i int) string {
		if idx[2*i] < 0 {
			return ""
		}
		return line[idx[2*i]:idx[2*i+1]]
	}
	quote := sub(2)
	if quote != sub(5) {
		return UsesMatch{}, false
	}
	a := sub(3)
	ref := sub(4)
	srcRef := sub(6)
	tail := sub(7)
	parsed, ok := action.ParseUsesValue(a, ref, srcRef, tail)
	if !ok {
		return UsesMatch{}, false
	}
	return UsesMatch{
		LineIndex: lineIdx,
		Start:     idx[3],
		End:       idx[1],
		Quote:     quote,
		Action:    parsed,
	}, true
}

// RenderUses builds the replacement string for the matched span.
// If srcRef is empty, no trailing comment is emitted (and tail is dropped).
func RenderUses(actionStr, ref, srcRef, tail, quote string) string {
	out := "uses: " + quote + actionStr + "@" + ref + quote
	if srcRef != "" {
		out += " # " + srcRef + tail
	}
	return out
}
