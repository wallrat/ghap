package cmd

import (
	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update [paths...]",
	Short: "Pin unpinned actions and refresh pinned ones in one pass",
	Long: `update brings every action up to the right commit. Per line:

  * Unpinned ('uses: owner/repo@<ref>'): resolve <ref> to a SHA and rewrite
    as 'uses: owner/repo@<sha> # <ref>' (same as 'ghap pin').

  * Pinned with a '# <ref>' source comment: re-resolve <ref> to its current
    commit SHA. If unchanged, the line is left alone.

  * Pinned without a source comment: bump to the latest release tag's SHA
    and annotate with '# <tag>'.

With --latest, every action (pinned or unpinned) is bumped to the latest
release tag's SHA and annotated with '# <tag>'.`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Usage()
		}
		interactive, _ := cmd.Flags().GetBool("interactive")
		latest, _ := cmd.Flags().GetBool("latest")
		mode := modeUpdate
		if latest {
			mode = modeLatest
		}
		return runMutate(args, mode, interactive)
	},
}

func init() {
	updateCmd.Flags().BoolP("interactive", "i", false, "confirm each change")
	updateCmd.Flags().Bool("latest", false, "bump every pinned action to its latest tag (removes # ref comments)")
}
