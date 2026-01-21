package cli

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"

	"ordiff/internal/cache"
	"ordiff/internal/github"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var CompareCmd = &cobra.Command{
	Use:   "compare <from> <to>",
	Short: "Compare two releases",
	Long: `Shows a comparison between two releases including commits, PRs, and file changes.

Example:
  ordiff compare v0.1.0 v0.2.0
  ordiff compare abc123 def456  # by commit SHA`,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		from := args[0]
		to := args[1]

		viper.SetConfigName(".ordiff")
		viper.SetConfigType("yaml")
		viper.AddConfigPath(".")
		if err := viper.ReadInConfig(); err != nil {
			if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
				log.Printf("Warning: could not read config: %v\n", err)
			}
		}

		owner := viper.GetString("default_owner")
		repo := viper.GetString("default_repo")

		if owner == "" || repo == "" {
			log.Fatal("No default repository. Run 'ordiff index <owner> <repo>' first.")
		}

		db, err := cache.NewDB("ordiff.db")
		if err != nil {
			log.Fatalf("Failed to open database: %v", err)
		}
		defer db.Close()

		fetcher := github.NewFetcher(owner, repo, nil)
		result, err := fetcher.GetCompareData(db, from, to)
		if err != nil {
			log.Fatalf("Failed to compare: %v", err)
		}

		if jsonOutput {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			enc.Encode(convertToJSON(result))
			return
		}

		printHumanOutput(result)
	},
}

func convertToJSON(r *github.CompareResult) map[string]interface{} {
	return map[string]interface{}{
		"from_release":  r.FromRelease.TagName,
		"to_release":    r.ToRelease.TagName,
		"commit_count":  len(r.Commits),
		"pr_count":      r.PrCount,
		"files_changed": len(r.Files),
		"commits":       r.Commits,
		"files":         r.Files,
	}
}

func printHumanOutput(r *github.CompareResult) {
	fmt.Printf("\n=== %s → %s ===\n\n", r.FromRelease.TagName, r.ToRelease.TagName)
	fmt.Printf("Commits: %d | PRs: %d | Files Changed: %d\n\n", len(r.Commits), r.PrCount, len(r.Files))

	if len(r.Files) > 0 {
		sort.Slice(r.Files, func(i, j int) bool {
			return r.Files[i].Changes > r.Files[j].Changes
		})

		fmt.Println("Top Changed Files:")
		fmt.Println("  +Add  -Del  File")
		fmt.Println("  ---- ----  ----")
		for _, f := range r.Files[:min(10, len(r.Files))] {
			fmt.Printf("  %+4d %-4d  %s\n", f.Additions, f.Deletions, f.Filename)
		}
		fmt.Println()
	}

	fmt.Println("Recent Commits:")
	for _, c := range r.Commits[:min(5, len(r.Commits))] {
		msg := c.Message
		if len(msg) > 60 {
			msg = msg[:57] + "..."
		}
		fmt.Printf("  %s  %s\n", c.SHA[:7], msg)
	}

	if len(r.Commits) > 5 {
		fmt.Printf("  ... and %d more commits\n", len(r.Commits)-5)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
