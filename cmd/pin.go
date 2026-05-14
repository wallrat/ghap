package cmd

import (
	"github.com/spf13/cobra"
)

var pinCmd = &cobra.Command{
	Use:   "pin [paths...]",
	Short: "Replace unpinned action refs with their resolved commit SHA",
	Long: `pin scans each workflow file for 'uses: owner/repo@<ref>' lines whose ref
is a branch or tag (not a 40-char commit SHA). The branch/tag is resolved to a
commit SHA via the GitHub API, and the line is rewritten as:

	uses: owner/repo@<sha> # <original-ref>

Already-pinned lines (ref is a SHA) are left alone.`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Usage()
		}
		f, _ := cmd.Flags().GetBool("interactive")
		return runMutate(args, modePin, f)
	},
}

func init() {
	pinCmd.Flags().BoolP("interactive", "i", false, "confirm each change")
}
