package store

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS jobs (
	id TEXT PRIMARY KEY,
	kind TEXT NOT NULL,
	format TEXT NOT NULL,
	status TEXT NOT NULL,
	error TEXT NOT NULL DEFAULT '',
	xml BLOB,
	json BLOB,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS current_document (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	xml BLOB NOT NULL,
	json BLOB NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS current_schema (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	root TEXT NOT NULL,
	xsd BLOB NOT NULL,
	json BLOB NOT NULL,
	updated_at TEXT NOT NULL
);
`

type Record struct {
	ID        string
	Kind      string
	Format    string
	Status    string
	Error     string
	XML       []byte
	JSON      []byte
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Store struct {
	db      *sql.DB
	maxJobs int
	mu      sync.Mutex
}

func Open(path string, maxJobs int) (*Store, error) {
	if maxJobs < 1 {
		maxJobs = 10_000
	}
	dsn, err := dsnFor(path)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetConnMaxIdleTime(0)
	if _, err := db.Exec(`PRAGMA busy_timeout=5000`); err != nil {
		db.Close()
		return nil, err
	}
	if !isMemoryDSN(dsn) {
		if _, err := db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
			db.Close()
			return nil, err
		}
		if _, err := db.Exec(`PRAGMA synchronous=NORMAL`); err != nil {
			db.Close()
			return nil, err
		}
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate sqlite: %w", err)
	}
	st := &Store{db: db, maxJobs: maxJobs}
	if err := st.failInterrupted(); err != nil {
		db.Close()
		return nil, err
	}
	return st, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) Put(rec Record) error {
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = time.Now()
	}
	if rec.UpdatedAt.IsZero() {
		rec.UpdatedAt = rec.CreatedAt
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.Exec(
		`INSERT INTO jobs (id, kind, format, status, error, xml, json, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
			kind=excluded.kind,
			format=excluded.format,
			status=excluded.status,
			error=excluded.error,
			xml=excluded.xml,
			json=excluded.json,
			updated_at=excluded.updated_at`,
		rec.ID, rec.Kind, rec.Format, rec.Status, rec.Error, rec.XML, rec.JSON,
		rec.CreatedAt.UTC().Format(time.RFC3339Nano),
		rec.UpdatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return err
	}
	if err := gcTx(tx, s.maxJobs); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Get(id string) (Record, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return scanOne(s.db.QueryRow(
		`SELECT id, kind, format, status, error, xml, json, created_at, updated_at FROM jobs WHERE id = ?`,
		id,
	))
}

func (s *Store) List() ([]Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(
		`SELECT id, kind, format, status, error, xml, json, created_at, updated_at
		 FROM jobs ORDER BY created_at DESC, id DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Record, 0)
	for rows.Next() {
		rec, err := scanRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`DELETE FROM jobs WHERE id = ?`, id)
	return err
}

func (s *Store) DeleteFinished(kind string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if kind == "" {
		_, err := s.db.Exec(`DELETE FROM jobs WHERE status IN ('done', 'failed')`)
		return err
	}
	_, err := s.db.Exec(`DELETE FROM jobs WHERE status IN ('done', 'failed') AND kind = ?`, kind)
	return err
}

func (s *Store) SetDocument(xmlBody, jsonBody []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(
		`INSERT INTO current_document (id, xml, json, updated_at) VALUES (1, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET xml=excluded.xml, json=excluded.json, updated_at=excluded.updated_at`,
		xmlBody, jsonBody, time.Now().UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *Store) Document() (xmlBody, jsonBody []byte, ok bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	err = s.db.QueryRow(`SELECT xml, json FROM current_document WHERE id = 1`).Scan(&xmlBody, &jsonBody)
	if err == sql.ErrNoRows {
		return nil, nil, false, nil
	}
	if err != nil {
		return nil, nil, false, err
	}
	return xmlBody, jsonBody, true, nil
}

func (s *Store) ClearDocument() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`DELETE FROM current_document`)
	return err
}

func (s *Store) SetSchema(root string, xsd, jsonBody []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(
		`INSERT INTO current_schema (id, root, xsd, json, updated_at) VALUES (1, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET root=excluded.root, xsd=excluded.xsd, json=excluded.json, updated_at=excluded.updated_at`,
		root, xsd, jsonBody, time.Now().UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *Store) Schema() (root string, xsd, jsonBody []byte, ok bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	err = s.db.QueryRow(`SELECT root, xsd, json FROM current_schema WHERE id = 1`).Scan(&root, &xsd, &jsonBody)
	if err == sql.ErrNoRows {
		return "", nil, nil, false, nil
	}
	if err != nil {
		return "", nil, nil, false, err
	}
	return root, xsd, jsonBody, true, nil
}

func (s *Store) ClearSchema() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`DELETE FROM current_schema`)
	return err
}

func (s *Store) failInterrupted() error {
	_, err := s.db.Exec(
		`UPDATE jobs SET status = 'failed', error = 'interrupted', xml = NULL, json = NULL, updated_at = ?
		 WHERE status IN ('queued', 'running')`,
		time.Now().UTC().Format(time.RFC3339Nano),
	)
	return err
}

func gcTx(tx *sql.Tx, maxJobs int) error {
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM jobs`).Scan(&n); err != nil {
		return err
	}
	for n > maxJobs {
		var id string
		err := tx.QueryRow(
			`SELECT id FROM jobs WHERE status IN ('done', 'failed') ORDER BY updated_at ASC, id ASC LIMIT 1`,
		).Scan(&id)
		if err == sql.ErrNoRows {
			return nil
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM jobs WHERE id = ?`, id); err != nil {
			return err
		}
		n--
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanOne(row rowScanner) (Record, bool, error) {
	rec, err := scanRow(row)
	if err == sql.ErrNoRows {
		return Record{}, false, nil
	}
	if err != nil {
		return Record{}, false, err
	}
	return rec, true, nil
}

func scanRow(row rowScanner) (Record, error) {
	var rec Record
	var created, updated string
	if err := row.Scan(&rec.ID, &rec.Kind, &rec.Format, &rec.Status, &rec.Error, &rec.XML, &rec.JSON, &created, &updated); err != nil {
		return Record{}, err
	}
	rec.CreatedAt = parseTime(created)
	rec.UpdatedAt = parseTime(updated)
	return rec, nil
}

func parseTime(s string) time.Time {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	return time.Time{}
}

func dsnFor(path string) (string, error) {
	if path == "" || path == ":memory:" {
		return fmt.Sprintf("file:tc-%d-%d?mode=memory&cache=shared", os.Getpid(), time.Now().UnixNano()), nil
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", fmt.Errorf("create store dir: %w", err)
		}
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}
	if !strings.HasPrefix(u.Path, "/") {
		u.Path = "/" + u.Path
	}
	return u.String() + "?_pragma=busy_timeout(5000)", nil
}

func isMemoryDSN(dsn string) bool {
	return strings.Contains(dsn, "mode=memory")
}
