package mcp

import (
	"encoding/json"
	"log"
	"os"
	"strconv"
	"sync"

	"ordiff/internal/cache"
	"ordiff/internal/github"

	"github.com/metoro-io/mcp-golang"
	"github.com/metoro-io/mcp-golang/transport/stdio"
	"github.com/spf13/viper"
)

type CompareArgs struct {
	From string `json:"from" jsonschema:"required,description=The older release tag or commit SHA"`
	To   string `json:"to" jsonschema:"required,description=The newer release tag or commit SHA"`
}

type ListReleasesArgs struct{}

type IndexArgs struct {
	Owner string `json:"owner" jsonschema:"required,description=The GitHub repository owner (e.g., 'ollama')"`
	Repo  string `json:"repo" jsonschema:"required,description=The GitHub repository name (e.g., 'ollama')"`
}

type ReleaseInfo struct {
	Tag    string `json:"tag"`
	Name   string `json:"name,omitempty"`
	Date   string `json:"date"`
	Commit string `json:"commit"`
}

type IndexStatus struct {
	Owner     string `json:"owner"`
	Repo      string `json:"repo"`
	IsRunning bool   `json:"is_running"`
	Progress  int    `json:"progress"`
	Total     int    `json:"total"`
	Message   string `json:"message"`
	Error     string `json:"error,omitempty"`
}

var (
	indexState struct {
		mu     sync.RWMutex
		status IndexStatus
	}
	dbInstance *cache.DB
)

func NewServer() *cache.DB {
	viper.SetConfigName(".ordiff")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			log.Printf("Warning: could not read config: %v\n", err)
		}
	}

	db, err := cache.NewDB("ordiff.db")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}

	dbInstance = db
	return db
}

func RunServer() {
	done := make(chan struct{})
	db := NewServer()

	go func() {
		defer db.Close()
		<-done
	}()

	server := mcp_golang.NewServer(stdio.NewStdioServerTransport())

	server.RegisterTool("index_repo", "Index a GitHub repository's releases and commits for caching", func(args IndexArgs) (*mcp_golang.ToolResponse, error) {
		owner := args.Owner
		repo := args.Repo

		if owner == "" || repo == "" {
			return mcp_golang.NewToolResponse(mcp_golang.NewTextContent("Error: owner and repo are required")), nil
		}

		indexState.mu.Lock()
		if indexState.status.IsRunning {
			indexState.mu.Unlock()
			return mcp_golang.NewToolResponse(mcp_golang.NewTextContent("Indexing already in progress for " + indexState.status.Owner + "/" + indexState.status.Repo + ". Use get_index_status to check progress.")), nil
		}
		indexState.status = IndexStatus{
			Owner:     owner,
			Repo:      repo,
			IsRunning: true,
			Progress:  0,
			Total:     100,
			Message:   "Starting indexing...",
		}
		indexState.mu.Unlock()

		token := os.Getenv("GITHUB_TOKEN")
		fetcher := github.NewFetcher(owner, repo, &token)

		go runIndexingAsync(owner, repo, fetcher, db)

		return mcp_golang.NewToolResponse(mcp_golang.NewTextContent("Started indexing " + owner + "/" + repo + ". Use get_index_status to check progress.")), nil
	})

	server.RegisterTool("get_index_status", "Get the status of the current indexing operation", func(args struct{}) (*mcp_golang.ToolResponse, error) {
		indexState.mu.RLock()
		defer indexState.mu.RUnlock()

		status := indexState.status
		if !status.IsRunning && status.Message == "" {
			return mcp_golang.NewToolResponse(mcp_golang.NewTextContent("No indexing in progress.")), nil
		}

		output := "Indexing Status:\n"
		output += "Owner: " + status.Owner + "\n"
		output += "Repo: " + status.Repo + "\n"
		output += "Status: " + map[bool]string{true: "Running", false: "Completed/Failed"}[status.IsRunning] + "\n"

		if status.Total > 0 {
			output += "Progress: " + strconv.Itoa(status.Progress) + "/" + strconv.Itoa(status.Total) + " (" + strconv.Itoa(status.Progress*100/status.Total) + "%)\n"
		}
		output += "Message: " + status.Message + "\n"
		if status.Error != "" {
			output += "Error: " + status.Error + "\n"
		}

		return mcp_golang.NewToolResponse(mcp_golang.NewTextContent(output)), nil
	})

	server.RegisterTool("list_releases", "List all cached releases for the default repository", func(args ListReleasesArgs) (*mcp_golang.ToolResponse, error) {
		owner := viper.GetString("default_owner")
		repo := viper.GetString("default_repo")

		if owner == "" || repo == "" {
			return mcp_golang.NewToolResponse(mcp_golang.NewTextContent("No default repository configured. Run 'ordiff index <owner> <repo>' first.")), nil
		}

		releases, err := db.GetReleases(owner, repo)
		if err != nil {
			return mcp_golang.NewToolResponse(mcp_golang.NewTextContent("Failed to list releases: " + err.Error())), nil
		}

		result := make([]ReleaseInfo, len(releases))
		for i, r := range releases {
			commit := r.CommitSHA
			if len(commit) > 7 {
				commit = commit[:7]
			}
			result[i] = ReleaseInfo{
				Tag:    r.TagName,
				Name:   r.Name,
				Date:   r.PublishedAt.Format("2006-01-02"),
				Commit: commit,
			}
		}

		return mcp_golang.NewToolResponse(mcp_golang.NewTextContent(formatReleases(result))), nil
	})

	server.RegisterTool("compare_releases", "Compare two releases and get detailed change information", func(args CompareArgs) (*mcp_golang.ToolResponse, error) {
		from := args.From
		to := args.To

		owner := viper.GetString("default_owner")
		repo := viper.GetString("default_repo")

		if owner == "" || repo == "" {
			return mcp_golang.NewToolResponse(mcp_golang.NewTextContent("No default repository configured. Run 'ordiff index <owner> <repo>' first.")), nil
		}

		token := os.Getenv("GITHUB_TOKEN")
		fetcher := github.NewFetcher(owner, repo, &token)

		result, err := fetcher.GetCompareData(db, from, to)
		if err != nil {
			return mcp_golang.NewToolResponse(mcp_golang.NewTextContent("Failed to compare: " + err.Error())), nil
		}

		output := formatCompareResult(result)
		return mcp_golang.NewToolResponse(mcp_golang.NewTextContent(output)), nil
	})

	server.RegisterTool("summarize_data", "Get structured data about release changes for AI summarization", func(args CompareArgs) (*mcp_golang.ToolResponse, error) {
		from := args.From
		to := args.To

		owner := viper.GetString("default_owner")
		repo := viper.GetString("default_repo")

		if owner == "" || repo == "" {
			return mcp_golang.NewToolResponse(mcp_golang.NewTextContent("No default repository configured. Run 'ordiff index <owner> <repo>' first.")), nil
		}

		token := os.Getenv("GITHUB_TOKEN")
		fetcher := github.NewFetcher(owner, repo, &token)

		result, err := fetcher.GetCompareData(db, from, to)
		if err != nil {
			return mcp_golang.NewToolResponse(mcp_golang.NewTextContent("Failed to summarize: " + err.Error())), nil
		}

		output := formatSummaryData(result)
		return mcp_golang.NewToolResponse(mcp_golang.NewTextContent(output)), nil
	})

	log.Println("Starting ordiff MCP server...")
	if err := server.Serve(); err != nil {
		log.Printf("Server error: %v\n", err)
		os.Exit(1)
	}
	<-done
}

func updateIndexProgress(progress, total int, message string) {
	indexState.mu.Lock()
	indexState.status.Progress = progress
	indexState.status.Total = total
	indexState.status.Message = message
	indexState.mu.Unlock()
}

func setIndexError(err string) {
	indexState.mu.Lock()
	indexState.status.IsRunning = false
	indexState.status.Error = err
	indexState.mu.Unlock()
}

func finishIndexing(success bool, message string) {
	indexState.mu.Lock()
	indexState.status.IsRunning = false
	indexState.status.Progress = indexState.status.Total
	indexState.status.Message = message
	if !success {
		indexState.status.Error = message
	}
	indexState.mu.Unlock()
}

func runIndexingAsync(owner, repo string, fetcher *github.Fetcher, db *cache.DB) {
	updateIndexProgress(0, 100, "Fetching releases...")

	releases, err := fetcher.FetchAllReleasesForIndexing(func(current, total int) {
		updateIndexProgress(current, total, "Fetching releases...")
	})
	if err != nil {
		setIndexError("Failed to fetch releases: " + err.Error())
		return
	}

	updateIndexProgress(20, 100, "Saving releases to cache...")
	for i, r := range releases {
		if err := db.SaveRelease(r); err != nil {
			log.Printf("Warning: failed to save release %s: %v\n", r.TagName, err)
		}
		updateIndexProgress(20+(i*10/len(releases)), 100, "Saving releases...")
	}

	updateIndexProgress(30, 100, "Fetching commits and files for missing release pairs...")

	totalPairs := len(releases) - 1
	processed := 0
	skipped := 0

	for i := 0; i < totalPairs; i++ {
		from := releases[i+1]
		to := releases[i]

		alreadyCached, _ := db.HasFileChangesCached(owner, repo, from.TagName, to.TagName)
		if alreadyCached {
			skipped++
			log.Printf("Skipping %s -> %s (already cached)\n", from.TagName, to.TagName)
			continue
		}

		processed++
		pendingPairs := totalPairs - skipped - processed
		updateIndexProgress(30+(processed*70/(processed+pendingPairs+1)), 100, "Processing "+from.TagName+" -> "+to.TagName+" ("+strconv.Itoa(processed)+" processed, "+strconv.Itoa(skipped)+" skipped)")

		commits, err := fetcher.FetchCommitsForIndexing(from.CommitSHA, to.CommitSHA, func(current, total int) {})
		if err != nil {
			log.Printf("Warning: failed to fetch commits: %v\n", err)
			continue
		}

		for _, c := range commits {
			if err := db.SaveCommit(c); err != nil {
				log.Printf("Warning: failed to save commit: %v\n", err)
			}
		}

		files, err := fetcher.FetchFileChangesForIndexing(from.CommitSHA, to.CommitSHA)
		if err != nil {
			log.Printf("Warning: failed to fetch files: %v\n", err)
			continue
		}

		for _, fc := range files {
			fc.FromRelease = from.TagName
			fc.ToRelease = to.TagName
			if err := db.SaveFileChange(fc); err != nil {
				log.Printf("Warning: failed to save file change: %v\n", err)
			}
		}
	}

	viper.Set("default_owner", owner)
	viper.Set("default_repo", repo)
	if err := viper.SafeWriteConfigAs(".ordiff.yaml"); err != nil {
		log.Printf("Warning: could not save config: %v\n", err)
	}

	finishIndexing(true, "Indexed "+owner+"/"+repo+" - "+strconv.Itoa(processed)+" new, "+strconv.Itoa(skipped)+" already cached")
}

func formatReleases(releases []ReleaseInfo) string {
	var output string
	for _, r := range releases {
		output += r.Tag + "  " + r.Date + "  " + r.Commit + "\n"
	}
	return output
}

func formatCompareResult(r *github.CompareResult) string {
	output := ""
	output += "=== " + r.FromRelease.TagName + " -> " + r.ToRelease.TagName + " ===\n\n"
	output += "Commits: " + strconv.Itoa(len(r.Commits)) + " | PRs: " + strconv.Itoa(r.PrCount) + " | Files: " + strconv.Itoa(len(r.Files)) + "\n\n"

	if len(r.Files) > 0 {
		output += "Top Changed Files:\n"
		output += "  +Add  -Del  File\n"
		output += "  ---- ----  ----\n"
		for i, f := range r.Files {
			if i >= 10 {
				break
			}
			output += "  " + strconv.Itoa(f.Additions) + "  " + strconv.Itoa(f.Deletions) + "  " + f.Filename + "\n"
		}
		output += "\n"
	}

	output += "Recent Commits:\n"
	for i, c := range r.Commits {
		if i >= 5 {
			break
		}
		msg := c.Message
		if len(msg) > 60 {
			msg = msg[:57] + "..."
		}
		sha := c.SHA
		if len(sha) > 7 {
			sha = sha[:7]
		}
		output += "  " + sha + "  " + msg + "\n"
	}

	if len(r.Commits) > 5 {
		output += "  ... and " + strconv.Itoa(len(r.Commits)-5) + " more commits\n"
	}

	return output
}

func formatSummaryData(r *github.CompareResult) string {
	type FileInfo struct {
		Name      string `json:"name"`
		Additions int    `json:"additions"`
		Deletions int    `json:"deletions"`
		Changes   int    `json:"changes"`
		Status    string `json:"status"`
	}

	type CommitInfo struct {
		SHA      string `json:"sha"`
		Message  string `json:"message"`
		Author   string `json:"author"`
		Date     string `json:"date"`
		PrNumber *int   `json:"pr_number,omitempty"`
	}

	type SummaryData struct {
		FromRelease  string       `json:"from_release"`
		ToRelease    string       `json:"to_release"`
		CommitCount  int          `json:"commit_count"`
		PrCount      int          `json:"pr_count"`
		FilesChanged int          `json:"files_changed"`
		TopFiles     []FileInfo   `json:"top_files"`
		Commits      []CommitInfo `json:"commits"`
	}

	maxFiles := len(r.Files)
	if maxFiles > 10 {
		maxFiles = 10
	}
	files := make([]FileInfo, maxFiles)
	for i := 0; i < maxFiles; i++ {
		f := r.Files[i]
		files[i] = FileInfo{
			Name:      f.Filename,
			Additions: f.Additions,
			Deletions: f.Deletions,
			Changes:   f.Changes,
			Status:    f.Status,
		}
	}

	maxCommits := len(r.Commits)
	if maxCommits > 20 {
		maxCommits = 20
	}
	commits := make([]CommitInfo, maxCommits)
	for i := 0; i < maxCommits; i++ {
		c := r.Commits[i]
		sha := c.SHA
		if len(sha) > 7 {
			sha = sha[:7]
		}
		commits[i] = CommitInfo{
			SHA:      sha,
			Message:  c.Message,
			Author:   c.Author,
			Date:     c.Date.Format("2006-01-02"),
			PrNumber: c.PrNumber,
		}
	}

	summary := SummaryData{
		FromRelease:  r.FromRelease.TagName,
		ToRelease:    r.ToRelease.TagName,
		CommitCount:  len(r.Commits),
		PrCount:      r.PrCount,
		FilesChanged: len(r.Files),
		TopFiles:     files,
		Commits:      commits,
	}

	b, err := json.Marshal(summary)
	if err != nil {
		return "{}"
	}
	return string(b)
}
