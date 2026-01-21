package github

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"

	"ordiff/internal/cache"

	"github.com/google/go-github/v81/github"
	"golang.org/x/oauth2"
)

type Fetcher struct {
	owner  string
	repo   string
	client *github.Client
	ctx    context.Context
}

func NewFetcher(owner, repo string, token *string) *Fetcher {
	var httpClient *http.Client
	if token != nil && *token != "" {
		ts := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: *token},
		)
		httpClient = oauth2.NewClient(context.Background(), ts)
	}
	return &Fetcher{
		owner:  owner,
		repo:   repo,
		client: github.NewClient(httpClient),
		ctx:    context.Background(),
	}
}

func (f *Fetcher) IndexAll(db *cache.DB) error {
	log.Printf("Fetching releases for %s/%s...\n", f.owner, f.repo)

	releases, err := f.fetchAllReleases()
	if err != nil {
		return fmt.Errorf("failed to fetch releases: %w", err)
	}

	log.Printf("Found %d releases, caching...\n", len(releases))

	for _, r := range releases {
		if err := db.SaveRelease(r); err != nil {
			return fmt.Errorf("failed to save release %s: %w", r.TagName, err)
		}
	}

	cachedPairs, _ := db.GetReleasePairCount(f.owner, f.repo)
	log.Printf("Already cached %d file change records\n", cachedPairs)

	log.Printf("Fetching commits and files for missing release pairs...\n")

	processed := 0
	skipped := 0
	for i := 0; i < len(releases)-1; i++ {
		from := releases[i+1]
		to := releases[i]

		alreadyCached, err := db.HasFileChangesCached(f.owner, f.repo, from.TagName, to.TagName)
		if err != nil {
			log.Printf("    Warning: failed to check cache: %v\n", err)
		}
		if alreadyCached {
			skipped++
			log.Printf("  Skipping %s → %s (already cached)\n", from.TagName, to.TagName)
			continue
		}

		processed++
		log.Printf("  Processing %s → %s (%d/%d, %d skipped)\n", from.TagName, to.TagName, processed, len(releases)-1-skipped, skipped)

		commits, err := f.fetchCommits(from.CommitSHA, to.CommitSHA)
		if err != nil {
			log.Printf("    Warning: failed to fetch commits: %v\n", err)
			continue
		}

		for _, c := range commits {
			if err := db.SaveCommit(c); err != nil {
				log.Printf("    Warning: failed to save commit: %v\n", err)
			}
		}

		files, err := f.fetchFileChanges(from.CommitSHA, to.CommitSHA)
		if err != nil {
			log.Printf("    Warning: failed to fetch files: %v\n", err)
			continue
		}

		for _, fc := range files {
			fc.FromRelease = from.TagName
			fc.ToRelease = to.TagName
			if err := db.SaveFileChange(fc); err != nil {
				log.Printf("    Warning: failed to save file change: %v\n", err)
			}
		}

		log.Println("    Sleeping 100ms to avoid rate limits...")
	}

	log.Printf("Indexing complete! Processed %d pairs, skipped %d already cached\n", processed, skipped)
	return nil
}

func (f *Fetcher) fetchAllReleases() ([]*cache.Release, error) {
	var allReleases []*cache.Release
	page := 1

	for {
		releases, resp, err := f.client.Repositories.ListReleases(f.ctx, f.owner, f.repo, &github.ListOptions{
			Page:    page,
			PerPage: 100,
		})
		if err != nil {
			return nil, err
		}

		for _, r := range releases {
			commitSHA := ""
			if r.TargetCommitish != nil && *r.TargetCommitish != "" {
				commitSHA = *r.TargetCommitish
			}

			release := &cache.Release{
				TagName:     r.GetTagName(),
				Name:        r.GetName(),
				PublishedAt: r.GetPublishedAt().Time,
				CommitSHA:   commitSHA,
				Body:        r.GetBody(),
				Owner:       f.owner,
				Repo:        f.repo,
			}
			allReleases = append(allReleases, release)
		}

		if resp.NextPage == 0 {
			break
		}
		page = resp.NextPage
	}

	return allReleases, nil
}

func (f *Fetcher) fetchCommits(fromSHA, toSHA string) ([]*cache.Commit, error) {
	if fromSHA == "" || toSHA == "" {
		return []*cache.Commit{}, nil
	}

	var allCommits []*cache.Commit
	page := 1

	for {
		commits, resp, err := f.client.Repositories.CompareCommits(f.ctx, f.owner, f.repo, fromSHA, toSHA, &github.ListOptions{
			Page:    page,
			PerPage: 100,
		})
		if err != nil {
			return nil, err
		}

		for _, c := range commits.Commits {
			commit := &cache.Commit{
				SHA:         c.GetSHA(),
				Message:     c.GetCommit().GetMessage(),
				Author:      c.GetCommit().GetAuthor().GetName(),
				AuthorEmail: c.GetCommit().GetAuthor().GetEmail(),
				Date:        c.GetCommit().GetAuthor().GetDate().Time,
				URL:         c.GetHTMLURL(),
				Owner:       f.owner,
				Repo:        f.repo,
			}

			prNum := f.extractPrNumber(c.GetCommit().GetMessage())
			if prNum != nil {
				commit.PrNumber = prNum
			}

			allCommits = append(allCommits, commit)
		}

		if resp.NextPage == 0 {
			break
		}
		page = resp.NextPage
	}

	return allCommits, nil
}

func (f *Fetcher) fetchFileChanges(fromSHA, toSHA string) ([]*cache.FileChange, error) {
	if fromSHA == "" || toSHA == "" {
		return []*cache.FileChange{}, nil
	}

	diff, _, err := f.client.Repositories.CompareCommits(f.ctx, f.owner, f.repo, fromSHA, toSHA, nil)
	if err != nil {
		return nil, err
	}

	var changes []*cache.FileChange
	for _, file := range diff.Files {
		change := &cache.FileChange{
			Filename:  file.GetFilename(),
			Additions: file.GetAdditions(),
			Deletions: file.GetDeletions(),
			Changes:   file.GetChanges(),
			Status:    file.GetStatus(),
			Patch:     file.GetPatch(),
			Owner:     f.owner,
			Repo:      f.repo,
		}
		changes = append(changes, change)
	}

	return changes, nil
}

func (f *Fetcher) extractPrNumber(msg string) *int {
	prefixes := []string{"#", "PR #", "pull/"}
	for _, prefix := range prefixes {
		idx := strings.Index(msg, prefix)
		if idx != -1 {
			rest := msg[idx+len(prefix):]
			numStr := ""
			for _, c := range rest {
				if c >= '0' && c <= '9' {
					numStr += string(c)
				} else if len(numStr) > 0 {
					break
				}
			}
			if len(numStr) > 0 {
				n := 0
				fmt.Sscanf(numStr, "%d", &n)
				return &n
			}
		}
	}
	return nil
}

func (f *Fetcher) GetCompareData(db *cache.DB, fromTag, toTag string) (*CompareResult, error) {
	fromRelease, err := db.GetRelease(f.owner, f.repo, fromTag)
	if err != nil {
		return nil, fmt.Errorf("release %s not found: %w", fromTag, err)
	}

	toRelease, err := db.GetRelease(f.owner, f.repo, toTag)
	if err != nil {
		return nil, fmt.Errorf("release %s not found: %w", toTag, err)
	}

	commits, err := db.GetCommitsBetween(f.owner, f.repo, fromTag, toTag)
	if err != nil {
		return nil, fmt.Errorf("failed to get commits: %w", err)
	}

	files, err := db.GetFileChanges(f.owner, f.repo, fromTag, toTag)
	if err != nil {
		return nil, fmt.Errorf("failed to get files: %w", err)
	}

	prCount, err := db.PrCountBetween(f.owner, f.repo, fromTag, toTag)
	if err != nil {
		return nil, fmt.Errorf("failed to count PRs: %w", err)
	}

	return &CompareResult{
		FromRelease: fromRelease,
		ToRelease:   toRelease,
		Commits:     commits,
		Files:       files,
		PrCount:     prCount,
	}, nil
}

func (f *Fetcher) FetchAllReleasesForIndexing(onProgress func(current, total int)) ([]*cache.Release, error) {
	var allReleases []*cache.Release
	page := 1
	totalPages := 0

	for {
		releases, resp, err := f.client.Repositories.ListReleases(f.ctx, f.owner, f.repo, &github.ListOptions{
			Page:    page,
			PerPage: 100,
		})
		if err != nil {
			return nil, err
		}

		for _, r := range releases {
			commitSHA := ""
			if r.TargetCommitish != nil && *r.TargetCommitish != "" {
				commitSHA = *r.TargetCommitish
			}

			release := &cache.Release{
				TagName:     r.GetTagName(),
				Name:        r.GetName(),
				PublishedAt: r.GetPublishedAt().Time,
				CommitSHA:   commitSHA,
				Body:        r.GetBody(),
				Owner:       f.owner,
				Repo:        f.repo,
			}
			allReleases = append(allReleases, release)
		}

		if resp.NextPage == 0 {
			break
		}
		if totalPages == 0 && resp.NextPage > page {
			totalPages = resp.NextPage
		}
		page = resp.NextPage

		if onProgress != nil {
			onProgress(page*100/totalPages, 100)
		}
	}

	return allReleases, nil
}

func (f *Fetcher) FetchCommitsForIndexing(fromSHA, toSHA string, onProgress func(current, total int)) ([]*cache.Commit, error) {
	if fromSHA == "" || toSHA == "" {
		return []*cache.Commit{}, nil
	}

	var allCommits []*cache.Commit
	page := 1
	totalPages := 0

	for {
		commits, resp, err := f.client.Repositories.CompareCommits(f.ctx, f.owner, f.repo, fromSHA, toSHA, &github.ListOptions{
			Page:    page,
			PerPage: 100,
		})
		if err != nil {
			return nil, err
		}

		for _, c := range commits.Commits {
			commit := &cache.Commit{
				SHA:         c.GetSHA(),
				Message:     c.GetCommit().GetMessage(),
				Author:      c.GetCommit().GetAuthor().GetName(),
				AuthorEmail: c.GetCommit().GetAuthor().GetEmail(),
				Date:        c.GetCommit().GetAuthor().GetDate().Time,
				URL:         c.GetHTMLURL(),
				Owner:       f.owner,
				Repo:        f.repo,
			}

			prNum := f.extractPrNumber(c.GetCommit().GetMessage())
			if prNum != nil {
				commit.PrNumber = prNum
			}

			allCommits = append(allCommits, commit)
		}

		if resp.NextPage == 0 {
			break
		}
		if totalPages == 0 && resp.NextPage > page {
			totalPages = resp.NextPage
		}
		page = resp.NextPage

		if onProgress != nil {
			onProgress(page*100/totalPages, 100)
		}
	}

	return allCommits, nil
}

func (f *Fetcher) FetchFileChangesForIndexing(fromSHA, toSHA string) ([]*cache.FileChange, error) {
	if fromSHA == "" || toSHA == "" {
		return []*cache.FileChange{}, nil
	}

	diff, _, err := f.client.Repositories.CompareCommits(f.ctx, f.owner, f.repo, fromSHA, toSHA, nil)
	if err != nil {
		return nil, err
	}

	var changes []*cache.FileChange
	for _, file := range diff.Files {
		change := &cache.FileChange{
			Filename:  file.GetFilename(),
			Additions: file.GetAdditions(),
			Deletions: file.GetDeletions(),
			Changes:   file.GetChanges(),
			Status:    file.GetStatus(),
			Patch:     file.GetPatch(),
			Owner:     f.owner,
			Repo:      f.repo,
		}
		changes = append(changes, change)
	}

	return changes, nil
}

type CompareResult struct {
	FromRelease *cache.Release
	ToRelease   *cache.Release
	Commits     []cache.Commit
	Files       []cache.FileChange
	PrCount     int
}
