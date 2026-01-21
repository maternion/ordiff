package cache

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	db *sql.DB
}

type Release struct {
	TagName     string
	Name        string
	PublishedAt time.Time
	CommitSHA   string
	Body        string
	Owner       string
	Repo        string
}

type Commit struct {
	SHA         string
	Message     string
	Author      string
	AuthorEmail string
	Date        time.Time
	URL         string
	Owner       string
	Repo        string
	PrNumber    *int
}

type PullRequest struct {
	Number   int
	Title    string
	Body     string
	State    string
	MergedAt *time.Time
	Author   string
	URL      string
	Owner    string
	Repo     string
}

type FileChange struct {
	Filename    string
	Additions   int
	Deletions   int
	Changes     int
	Status      string
	Patch       string
	Owner       string
	Repo        string
	FromRelease string
	ToRelease   string
}

func NewDB(path string) (*DB, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open db: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping db: %w", err)
	}

	if err := initSchema(db); err != nil {
		return nil, fmt.Errorf("failed to init schema: %w", err)
	}

	return &DB{db: db}, nil
}

func (d *DB) Close() error {
	return d.db.Close()
}

func initSchema(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS releases (
		tag_name TEXT PRIMARY KEY,
		name TEXT,
		published_at TEXT,
		commit_sha TEXT,
		body TEXT,
		owner TEXT,
		repo TEXT
	);

	CREATE TABLE IF NOT EXISTS commits (
		sha TEXT PRIMARY KEY,
		message TEXT,
		author TEXT,
		author_email TEXT,
		date TEXT,
		url TEXT,
		owner TEXT,
		repo TEXT,
		pr_number INTEGER
	);

	CREATE TABLE IF NOT EXISTS pull_requests (
		number INTEGER,
		title TEXT,
		body TEXT,
		state TEXT,
		merged_at TEXT,
		author TEXT,
		url TEXT,
		owner TEXT,
		repo TEXT,
		PRIMARY KEY (owner, repo, number)
	);

	CREATE TABLE IF NOT EXISTS file_changes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		filename TEXT,
		additions INTEGER,
		deletions INTEGER,
		changes INTEGER,
		status TEXT,
		patch TEXT,
		owner TEXT,
		repo TEXT,
		from_release TEXT,
		to_release TEXT
	);

	CREATE INDEX IF NOT EXISTS idx_commits_owner_repo ON commits(owner, repo);
	CREATE INDEX IF NOT EXISTS idx_prs_owner_repo ON pull_requests(owner, repo);
	CREATE INDEX IF NOT EXISTS idx_files_release ON file_changes(owner, repo, from_release, to_release);
	`

	_, err := db.Exec(schema)
	return err
}

func (d *DB) SaveRelease(r *Release) error {
	_, err := d.db.Exec(`
		INSERT OR REPLACE INTO releases (tag_name, name, published_at, commit_sha, body, owner, repo)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, r.TagName, r.Name, r.PublishedAt.Format(time.RFC3339), r.CommitSHA, r.Body, r.Owner, r.Repo)
	return err
}

func (d *DB) SaveCommit(c *Commit) error {
	var prNum interface{}
	if c.PrNumber != nil {
		prNum = *c.PrNumber
	}
	_, err := d.db.Exec(`
		INSERT OR REPLACE INTO commits (sha, message, author, author_email, date, url, owner, repo, pr_number)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, c.SHA, c.Message, c.Author, c.AuthorEmail, c.Date.Format(time.RFC3339), c.URL, c.Owner, c.Repo, prNum)
	return err
}

func (d *DB) SavePullRequest(pr *PullRequest) error {
	var mergedAt interface{}
	if pr.MergedAt != nil {
		mergedAt = pr.MergedAt.Format(time.RFC3339)
	}
	_, err := d.db.Exec(`
		INSERT OR REPLACE INTO pull_requests (number, title, body, state, merged_at, author, url, owner, repo)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, pr.Number, pr.Title, pr.Body, pr.State, mergedAt, pr.Author, pr.URL, pr.Owner, pr.Repo)
	return err
}

func (d *DB) SaveFileChange(fc *FileChange) error {
	_, err := d.db.Exec(`
		INSERT INTO file_changes (filename, additions, deletions, changes, status, patch, owner, repo, from_release, to_release)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, fc.Filename, fc.Additions, fc.Deletions, fc.Changes, fc.Status, fc.Patch, fc.Owner, fc.Repo, fc.FromRelease, fc.ToRelease)
	return err
}

func (d *DB) GetReleases(owner, repo string) ([]Release, error) {
	rows, err := d.db.Query(`
		SELECT tag_name, name, published_at, commit_sha, body
		FROM releases
		WHERE owner = ? AND repo = ?
		ORDER BY published_at DESC
	`, owner, repo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var releases []Release
	for rows.Next() {
		var r Release
		var publishedAt string
		if err := rows.Scan(&r.TagName, &r.Name, &publishedAt, &r.CommitSHA, &r.Body); err != nil {
			return nil, err
		}
		r.Owner = owner
		r.Repo = repo
		r.PublishedAt, _ = time.Parse(time.RFC3339, publishedAt)
		releases = append(releases, r)
	}
	return releases, rows.Err()
}

func (d *DB) GetRelease(owner, repo, tag string) (*Release, error) {
	var r Release
	var publishedAt string
	err := d.db.QueryRow(`
		SELECT tag_name, name, published_at, commit_sha, body
		FROM releases
		WHERE owner = ? AND repo = ? AND tag_name = ?
	`, owner, repo, tag).Scan(&r.TagName, &r.Name, &publishedAt, &r.CommitSHA, &r.Body)
	if err != nil {
		return nil, err
	}
	r.Owner = owner
	r.Repo = repo
	r.PublishedAt, _ = time.Parse(time.RFC3339, publishedAt)
	return &r, nil
}

func (d *DB) GetCommitsBetween(owner, repo, fromTag, toTag string) ([]Commit, error) {
	rows, err := d.db.Query(`
		SELECT c.sha, c.message, c.author, c.author_email, c.date, c.url, c.pr_number
		FROM commits c
		JOIN releases r1 ON c.date >= r1.published_at
		JOIN releases r2 ON c.date <= r2.published_at
		WHERE c.owner = ? AND c.repo = ?
		AND r1.tag_name = ? AND r2.tag_name = ?
		ORDER BY c.date ASC
	`, owner, repo, fromTag, toTag)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var commits []Commit
	for rows.Next() {
		var c Commit
		var prNum *int
		var date string
		if err := rows.Scan(&c.SHA, &c.Message, &c.Author, &c.AuthorEmail, &date, &c.URL, &prNum); err != nil {
			return nil, err
		}
		c.PrNumber = prNum
		c.Owner = owner
		c.Repo = repo
		c.Date, _ = time.Parse(time.RFC3339, date)
		commits = append(commits, c)
	}
	return commits, rows.Err()
}

func (d *DB) GetFileChanges(owner, repo, fromTag, toTag string) ([]FileChange, error) {
	rows, err := d.db.Query(`
		SELECT filename, additions, deletions, changes, status, patch
		FROM file_changes
		WHERE owner = ? AND repo = ? AND from_release = ? AND to_release = ?
	`, owner, repo, fromTag, toTag)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var changes []FileChange
	for rows.Next() {
		var fc FileChange
		if err := rows.Scan(&fc.Filename, &fc.Additions, &fc.Deletions, &fc.Changes, &fc.Status, &fc.Patch); err != nil {
			return nil, err
		}
		fc.Owner = owner
		fc.Repo = repo
		fc.FromRelease = fromTag
		fc.ToRelease = toTag
		changes = append(changes, fc)
	}
	return changes, rows.Err()
}

func (d *DB) PrCountBetween(owner, repo, fromTag, toTag string) (int, error) {
	var count int
	err := d.db.QueryRow(`
		SELECT COUNT(DISTINCT c.pr_number)
		FROM commits c
		JOIN releases r1 ON c.date >= r1.published_at
		JOIN releases r2 ON c.date <= r2.published_at
		WHERE c.owner = ? AND c.repo = ? AND c.pr_number IS NOT NULL
		AND r1.tag_name = ? AND r2.tag_name = ?
	`, owner, repo, fromTag, toTag).Scan(&count)
	return count, err
}

func (d *DB) HasFileChangesCached(owner, repo, fromRelease, toRelease string) (bool, error) {
	var count int
	err := d.db.QueryRow(`
		SELECT COUNT(*) FROM file_changes
		WHERE owner = ? AND repo = ? AND from_release = ? AND to_release = ?
	`, owner, repo, fromRelease, toRelease).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (d *DB) GetReleasePairCount(owner, repo string) (int, error) {
	var count int
	err := d.db.QueryRow(`
		SELECT COUNT(*) FROM file_changes
		WHERE owner = ? AND repo = ?
	`, owner, repo).Scan(&count)
	return count, err
}
