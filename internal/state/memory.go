package state

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// MemoryDB is a SQLite-backed time-series memory store for a single book.
// It supports relevance-based retrieval of facts, hooks, and summaries.
type MemoryDB struct {
	db *sql.DB
}

// MemoryEntry is a single record in the memory database.
type MemoryEntry struct {
	ID        int64
	BookID    string
	Chapter   int
	Kind      string // fact/hook/summary/arc/resource
	Subject   string
	Content   string
	Tags      string // comma-separated
	CreatedAt time.Time
}

// MemoryQuery specifies retrieval parameters.
type MemoryQuery struct {
	BookID  string
	Chapter int    // context chapter (for recency scoring)
	Kind    string // filter by kind; empty = all
	Terms   []string
	Limit   int
}

// OpenMemoryDB opens (or creates) the memory database at dbPath.
func OpenMemoryDB(dbPath string) (*MemoryDB, error) {
	if err := ensureDir(filepath.Dir(dbPath)); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1) // SQLite is single-writer
	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &MemoryDB{db: db}, nil
}

// Close closes the database connection.
func (m *MemoryDB) Close() error {
	return m.db.Close()
}

// Insert adds a new memory entry.
func (m *MemoryDB) Insert(ctx context.Context, e MemoryEntry) error {
	_, err := m.db.ExecContext(ctx,
		`INSERT INTO memory (book_id, chapter, kind, subject, content, tags, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		e.BookID, e.Chapter, e.Kind, e.Subject, e.Content, e.Tags, e.CreatedAt.UTC(),
	)
	return err
}

// Recall retrieves entries matching the query, ordered by relevance (recency + term match).
func (m *MemoryDB) Recall(ctx context.Context, q MemoryQuery) ([]MemoryEntry, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = 20
	}

	var args []any
	conds := []string{"book_id = ?"}
	args = append(args, q.BookID)

	if q.Kind != "" {
		conds = append(conds, "kind = ?")
		args = append(args, q.Kind)
	}

	if len(q.Terms) > 0 {
		termConds := make([]string, len(q.Terms))
		for i, t := range q.Terms {
			termConds[i] = "(subject LIKE ? OR content LIKE ? OR tags LIKE ?)"
			like := "%" + t + "%"
			args = append(args, like, like, like)
		}
		conds = append(conds, "("+strings.Join(termConds, " OR ")+")")
	}

	query := fmt.Sprintf(
		`SELECT id, book_id, chapter, kind, subject, content, tags, created_at
		 FROM memory WHERE %s
		 ORDER BY ABS(chapter - ?) ASC, id DESC
		 LIMIT ?`,
		strings.Join(conds, " AND "),
	)
	args = append(args, q.Chapter, limit)

	rows, err := m.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []MemoryEntry
	for rows.Next() {
		var e MemoryEntry
		var createdAt string
		if err := rows.Scan(&e.ID, &e.BookID, &e.Chapter, &e.Kind, &e.Subject, &e.Content, &e.Tags, &createdAt); err != nil {
			return nil, err
		}
		e.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// DeleteFrom removes all entries for a book at chapter >= fromChapter.
func (m *MemoryDB) DeleteFrom(ctx context.Context, bookID string, fromChapter int) error {
	_, err := m.db.ExecContext(ctx,
		`DELETE FROM memory WHERE book_id = ? AND chapter >= ?`,
		bookID, fromChapter,
	)
	return err
}

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS memory (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			book_id    TEXT    NOT NULL,
			chapter    INTEGER NOT NULL,
			kind       TEXT    NOT NULL,
			subject    TEXT    NOT NULL DEFAULT '',
			content    TEXT    NOT NULL DEFAULT '',
			tags       TEXT    NOT NULL DEFAULT '',
			created_at TEXT    NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_memory_book_chapter ON memory(book_id, chapter);
		CREATE INDEX IF NOT EXISTS idx_memory_kind ON memory(book_id, kind);
	`)
	return err
}

func ensureDir(dir string) error {
	if dir == "" || dir == "." {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}
