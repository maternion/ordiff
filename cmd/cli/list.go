package cli

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"ordiff/internal/cache"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var jsonOutput bool

var ListCmd = &cobra.Command{
	Use:   "list",
	Short: "List cached releases",
	Long: `Displays all releases that have been indexed for the default repository.

Example:
  ordiff list`,
	Run: func(cmd *cobra.Command, args []string) {
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

		releases, err := db.GetReleases(owner, repo)
		if err != nil {
			log.Fatalf("Failed to get releases: %v", err)
		}

		if jsonOutput {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			enc.Encode(releases)
			return
		}

		fmt.Printf("Releases for %s/%s:\n\n", owner, repo)
		for _, r := range releases {
			fmt.Printf("  %-20s  %s\n", r.TagName, r.PublishedAt.Format("2006-01-02"))
		}
	},
}

func init() {
	ListCmd.Flags().BoolVarP(&jsonOutput, "json", "j", false, "Output as JSON")
}
