package cli

import (
	"fmt"
	"log"

	"ordiff/internal/cache"
	"ordiff/internal/github"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var IndexCmd = &cobra.Command{
	Use:   "index <owner> <repo>",
	Short: "Index a GitHub repository's releases and commits",
	Long: `Fetches all releases, commits, PRs and file changes from a GitHub repository
and stores them in a local SQLite cache for fast comparisons.

Example:
  ordiff index ollama ollama`,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		owner := args[0]
		repo := args[1]

		fmt.Printf("Indexing %s/%s...\n", owner, repo)

		db, err := cache.NewDB("ordiff.db")
		if err != nil {
			log.Fatalf("Failed to open database: %v", err)
		}
		defer db.Close()

		fetcher := github.NewFetcher(owner, repo, nil)
		if err := fetcher.IndexAll(db); err != nil {
			log.Fatalf("Failed to index: %v", err)
		}

		viper.Set("default_owner", owner)
		viper.Set("default_repo", repo)
		if err := viper.WriteConfig(); err != nil {
			if _, ok := err.(viper.ConfigFileNotFoundError); ok {
				viper.SafeWriteConfigAs(".ordiff.yaml")
			} else {
				log.Printf("Warning: could not save config: %v\n", err)
			}
		}

		fmt.Println("Indexing complete!")
		fmt.Printf("Run 'ordiff list' to see releases.\n")
	},
}
