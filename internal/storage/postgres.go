package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/shairozan/PanelTree/internal/workspace"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type Postgres struct{ dsn, root string }

var identifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)

func Connect(ctx context.Context, dsn, root string) (*Postgres, error) {
	if dsn == "" || root == "" {
		return nil, fmt.Errorf("PostgreSQL DSN and blob-root are required")
	}
	root, e := filepath.Abs(root)
	if e != nil {
		return nil, e
	}
	if e = os.MkdirAll(root, 0700); e != nil {
		return nil, e
	}
	p := &Postgres{dsn: dsn, root: root}
	c, e := p.connect(ctx)
	if e != nil {
		return nil, e
	}
	_ = c.Close(ctx)
	return p, nil
}
func (*Postgres) Close() {}
func (p *Postgres) connect(ctx context.Context) (*pgx.Conn, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	c, e := pgx.Connect(ctx, p.dsn)
	if e != nil {
		return nil, fmt.Errorf("PostgreSQL connection failed (check host DSN environment and server)")
	}
	return c, nil
}
func (p *Postgres) Migrate(ctx context.Context) error {
	c, e := p.connect(ctx)
	if e != nil {
		return e
	}
	defer func() { _ = c.Close(context.Background()) }()
	tx, e := c.Begin(ctx)
	if e != nil {
		return e
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(250025)`); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS pt_schema(version integer NOT NULL)`); e != nil {
		return e
	}
	var version int
	e = tx.QueryRow(ctx, `SELECT version FROM pt_schema`).Scan(&version)
	if e == nil {
		if version < 1 || version > 3 {
			return fmt.Errorf("unsupported storage schema version %d", version)
		}
		if version == 1 {
			if e = migrateJobs(ctx, tx); e != nil {
				return e
			}
		}
		if version < 3 {
			if e = migrateLibrary(ctx, tx); e != nil {
				return e
			}
		}
		return tx.Commit(ctx)
	}
	if e != pgx.ErrNoRows {
		return e
	}
	_, e = tx.Exec(ctx, `CREATE TABLE pt_projects(id text PRIMARY KEY, revision bigint NOT NULL DEFAULT 0);
 CREATE TABLE pt_documents(project_id text REFERENCES pt_projects(id), path text, ordinal integer NOT NULL, payload jsonb NOT NULL, PRIMARY KEY(project_id,path), UNIQUE(project_id,ordinal));
 CREATE TABLE pt_blobs(hash text PRIMARY KEY CHECK(length(hash)=64), size bigint NOT NULL);
 CREATE TABLE pt_files(project_id text REFERENCES pt_projects(id),path text,hash text REFERENCES pt_blobs(hash),PRIMARY KEY(project_id,path));
 CREATE TABLE pt_metadata(project_id text REFERENCES pt_projects(id),path text,payload jsonb NOT NULL,original bytea NOT NULL,PRIMARY KEY(project_id,path));
 INSERT INTO pt_schema VALUES(1);`)
	if e != nil {
		return e
	}
	if e = migrateJobs(ctx, tx); e != nil {
		return e
	}
	if e = migrateLibrary(ctx, tx); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func (p *Postgres) check(ctx context.Context, c *pgx.Conn) error {
	var v int
	if e := c.QueryRow(ctx, `SELECT version FROM pt_schema`).Scan(&v); e != nil {
		return fmt.Errorf("storage schema unavailable; run storage migrate")
	}
	if v != 3 {
		return fmt.Errorf("unsupported storage schema version %d", v)
	}
	return nil
}
func (p *Postgres) put(data []byte) (string, error) {
	hash := digest(data)
	dir := filepath.Join(p.root, "blobs")
	if e := os.MkdirAll(dir, 0700); e != nil {
		return "", e
	}
	path, e := workspace.SafePath(dir, hash)
	if e != nil {
		return "", e
	}
	if prior, e := os.ReadFile(path); e == nil {
		if digest(prior) != hash {
			return "", fmt.Errorf("corrupt canonical blob")
		}
		return hash, nil
	} else if !os.IsNotExist(e) {
		return "", e
	}
	f, e := os.CreateTemp(dir, ".stage-")
	if e != nil {
		return "", e
	}
	name := f.Name()
	defer func() { _ = os.Remove(name) }()
	if _, e = f.Write(data); e != nil {
		_ = f.Close()
		return "", e
	}
	if e = f.Sync(); e != nil {
		_ = f.Close()
		return "", e
	}
	if e = f.Close(); e != nil {
		return "", e
	}
	if e = os.Link(name, path); e != nil {
		prior, readErr := os.ReadFile(path)
		if readErr != nil || digest(prior) != hash {
			return "", e
		}
	}
	return hash, nil
}
func (p *Postgres) get(hash string) ([]byte, error) {
	if !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(hash) {
		return nil, fmt.Errorf("invalid blob hash")
	}
	path, e := workspace.SafePath(filepath.Join(p.root, "blobs"), hash)
	if e != nil {
		return nil, e
	}
	b, e := os.ReadFile(path)
	if e != nil {
		return nil, fmt.Errorf("missing blob %s", hash)
	}
	if digest(b) != hash {
		return nil, fmt.Errorf("corrupt blob %s", hash)
	}
	return b, nil
}
func (p *Postgres) save(ctx context.Context, tx pgx.Tx, id string, b bundle) error {
	for _, table := range []string{"pt_documents", "pt_files", "pt_metadata"} {
		if _, e := tx.Exec(ctx, `DELETE FROM `+table+` WHERE project_id=$1`, id); e != nil {
			return e
		}
	}
	for i, d := range b.Documents {
		if _, e := tx.Exec(ctx, `INSERT INTO pt_documents VALUES($1,$2,$3,$4)`, id, d.Path, i, documentJSON(d.Value)); e != nil {
			return e
		}
	}
	for path, data := range b.Files {
		if path == ".paneltree/state.json" || strings.HasPrefix(path, ".paneltree/references/") {
			if !json.Valid(data) {
				return fmt.Errorf("invalid metadata %s", path)
			}
			if _, e := tx.Exec(ctx, `INSERT INTO pt_metadata VALUES($1,$2,$3,$4)`, id, path, json.RawMessage(data), data); e != nil {
				return e
			}
			continue
		}
		hash, e := p.put(data)
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO pt_blobs VALUES($1,$2) ON CONFLICT DO NOTHING`, hash, len(data)); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO pt_files VALUES($1,$2,$3)`, id, path, hash); e != nil {
			return e
		}
	}
	if e := p.restoreLibrary(ctx, tx, id, b.Library, b.Files); e != nil {
		return e
	}
	_, e := tx.Exec(ctx, `UPDATE pt_projects SET revision=revision+1 WHERE id=$1`, id)
	return e
}
func (p *Postgres) load(ctx context.Context, c *pgx.Conn, id string) (bundle, error) {
	b := bundle{Files: map[string][]byte{}}
	var exists bool
	if e := c.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pt_projects WHERE id=$1)`, id).Scan(&exists); e != nil {
		return b, e
	}
	if !exists {
		return b, fmt.Errorf("unknown PostgreSQL project %s", id)
	}
	rows, e := c.Query(ctx, `SELECT path,payload FROM pt_documents WHERE project_id=$1 ORDER BY ordinal`, id)
	if e != nil {
		return b, e
	}
	for rows.Next() {
		var d document
		var data []byte
		if e = rows.Scan(&d.Path, &data); e != nil {
			rows.Close()
			return b, e
		}
		if e = json.Unmarshal(data, &d.Value); e != nil {
			rows.Close()
			return b, e
		}
		b.Documents = append(b.Documents, d)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return b, e
	}
	rows, e = c.Query(ctx, `SELECT path,hash FROM pt_files WHERE project_id=$1`, id)
	if e != nil {
		return b, e
	}
	for rows.Next() {
		var path, hash string
		if e = rows.Scan(&path, &hash); e != nil {
			break
		}
		b.Files[path], e = p.get(hash)
		if e != nil {
			break
		}
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	if e != nil {
		return b, e
	}
	rows, e = c.Query(ctx, `SELECT path,original FROM pt_metadata WHERE project_id=$1`, id)
	if e != nil {
		return b, e
	}
	defer rows.Close()
	for rows.Next() {
		var path string
		var data []byte
		if e = rows.Scan(&path, &data); e != nil {
			return b, e
		}
		b.Files[path] = data
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return b, e
	}
	b.Library, e = p.projectLibrary(ctx, c, id)
	return b, e
}
func (p *Postgres) Import(ctx context.Context, id, entry string) error {
	if !identifier.MatchString(id) {
		return fmt.Errorf("invalid project ID")
	}
	return workspace.Open(ctx, entry, func(w *workspace.Session) error {
		b, e := capture(w.Owner)
		if e != nil {
			return e
		}
		c, e := p.connect(ctx)
		if e != nil {
			return e
		}
		defer func() { _ = c.Close(context.Background()) }()
		if e = p.check(ctx, c); e != nil {
			return e
		}
		tx, e := c.Begin(ctx)
		if e != nil {
			return e
		}
		defer func() { _ = tx.Rollback(context.Background()) }()
		if _, e = tx.Exec(ctx, `INSERT INTO pt_projects(id) VALUES($1)`, id); e != nil {
			return fmt.Errorf("project ID already exists or import failed")
		}
		if e = p.save(ctx, tx, id, b); e != nil {
			return e
		}
		if e = p.importJobs(ctx, tx, id, b.Jobs); e != nil {
			return e
		}
		return tx.Commit(ctx)
	})
}
func (p *Postgres) Open(ctx context.Context, id string, fn func(*workspace.Session) error) error {
	return p.open(ctx, id, fn, nil)
}
func (p *Postgres) open(ctx context.Context, id string, fn func(*workspace.Session) error, extra func(pgx.Tx) error) error {
	if !identifier.MatchString(id) {
		return fmt.Errorf("invalid project ID")
	}
	c, e := p.connect(ctx)
	if e != nil {
		return e
	}
	defer func() { _ = c.Close(context.Background()) }()
	if e = p.check(ctx, c); e != nil {
		return e
	}
	if _, e = c.Exec(ctx, `SELECT pg_advisory_lock(hashtextextended($1,25))`, id); e != nil {
		return e
	}
	b, e := p.load(ctx, c, id)
	if e != nil {
		return e
	}
	dir, e := os.MkdirTemp("", "paneltree-pg-")
	if e != nil {
		return e
	}
	defer func() { _ = os.RemoveAll(dir) }()
	if e = materialize(dir, b); e != nil {
		return e
	}
	if e = workspace.Open(ctx, filepath.Join(dir, "project.yaml"), fn); e != nil {
		return e
	}
	next, e := capture(filepath.Join(dir, "project.yaml"))
	if e != nil {
		return e
	}
	before, _ := json.Marshal(b)
	after, _ := json.Marshal(next)
	if string(before) == string(after) && extra == nil {
		return nil
	}
	tx, e := c.Begin(ctx)
	if e != nil {
		return e
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if e = p.save(ctx, tx, id, next); e != nil {
		return e
	}
	if extra != nil {
		if e = extra(tx); e != nil {
			return e
		}
	}
	return tx.Commit(ctx)
}
func (p *Postgres) Export(ctx context.Context, id, dest string) error {
	return p.Open(ctx, id, func(w *workspace.Session) error {
		b, e := capture(w.Owner)
		if e != nil {
			return e
		}
		b.Jobs, e = p.exportJobs(ctx, id)
		if e != nil {
			return e
		}
		return publishDirectory(dest, b)
	})
}

func (p *Postgres) Projects(ctx context.Context) ([]string, error) {
	c, e := p.readyConnect(ctx)
	if e != nil {
		return nil, e
	}
	defer func() { _ = c.Close(context.Background()) }()
	rows, e := c.Query(ctx, `SELECT id FROM pt_projects ORDER BY id`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			return nil, e
		}
		out = append(out, "pg:"+id)
	}
	return out, rows.Err()
}

// readyConnect gates service operations against the adapter's supported schema.
// Migration and initial connectivity checks deliberately use connect instead.
func (p *Postgres) readyConnect(ctx context.Context) (*pgx.Conn, error) {
	c, e := p.connect(ctx)
	if e != nil {
		return nil, e
	}
	if e = p.check(ctx, c); e != nil {
		_ = c.Close(context.Background())
		return nil, e
	}
	return c, nil
}
