package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/shairozan/PanelTree/internal/workspace"
	"os"
	"path/filepath"
)

type LibraryVersion struct {
	ID               string            `json:"id"`
	Version          string            `json:"version"`
	Package          string            `json:"package"`
	ReferenceSet     string            `json:"reference_set,omitempty"`
	ReferenceVersion string            `json:"reference_version,omitempty"`
	Files            map[string]string `json:"files"`
}

func migrateLibrary(ctx context.Context, tx pgx.Tx) error {
	_, e := tx.Exec(ctx, `CREATE TABLE pt_library(id text,version text,payload jsonb NOT NULL,PRIMARY KEY(id,version));CREATE TABLE pt_library_assets(id text,version text,path text,hash text REFERENCES pt_blobs(hash),PRIMARY KEY(id,version,path),FOREIGN KEY(id,version) REFERENCES pt_library(id,version) ON DELETE CASCADE);CREATE TABLE pt_bindings(project_id text REFERENCES pt_projects(id),package_path text,id text,version text,PRIMARY KEY(project_id,package_path),FOREIGN KEY(id,version) REFERENCES pt_library(id,version));UPDATE pt_schema SET version=3;`)
	return e
}
func (p *Postgres) PublishLibrary(ctx context.Context, v LibraryVersion, files map[string][]byte) error {
	if !identifier.MatchString(v.ID) || !identifier.MatchString(v.Version) || len(files) == 0 || len(files[v.Package]) == 0 {
		return fmt.Errorf("library ID, version and package files required")
	}
	v.Files = map[string]string{}
	for path, data := range files {
		if _, e := workspace.SafePath(p.root, path); e != nil {
			return e
		}
		hash, e := p.put(data)
		if e != nil {
			return e
		}
		v.Files[path] = hash
	}
	c, e := p.readyConnect(ctx)
	if e != nil {
		return e
	}
	defer func() { _ = c.Close(context.Background()) }()
	tx, e := c.Begin(ctx)
	if e != nil {
		return e
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	data, e := json.Marshal(v)
	if e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO pt_library VALUES($1,$2,$3)`, v.ID, v.Version, data); e != nil {
		return fmt.Errorf("library version already exists or publication failed")
	}
	for path, hash := range v.Files {
		if _, e = tx.Exec(ctx, `INSERT INTO pt_blobs VALUES($1,$2) ON CONFLICT DO NOTHING`, hash, len(files[path])); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO pt_library_assets VALUES($1,$2,$3,$4)`, v.ID, v.Version, path, hash); e != nil {
			return e
		}
	}
	return tx.Commit(ctx)
}
func (p *Postgres) Library(ctx context.Context) ([]LibraryVersion, error) {
	c, e := p.readyConnect(ctx)
	if e != nil {
		return nil, e
	}
	defer func() { _ = c.Close(context.Background()) }()
	rows, e := c.Query(ctx, `SELECT payload FROM pt_library ORDER BY id,version`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []LibraryVersion{}
	for rows.Next() {
		var data []byte
		var v LibraryVersion
		if e = rows.Scan(&data); e != nil {
			return nil, e
		}
		if e = json.Unmarshal(data, &v); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (p *Postgres) BindLibrary(ctx context.Context, project, id, version string, fn func(*workspace.Session, LibraryVersion) error) error {
	c, e := p.readyConnect(ctx)
	if e != nil {
		return e
	}
	defer func() { _ = c.Close(context.Background()) }()
	var data []byte
	if e = c.QueryRow(ctx, `SELECT payload FROM pt_library WHERE id=$1 AND version=$2`, id, version).Scan(&data); e != nil {
		return fmt.Errorf("library version unavailable")
	}
	var v LibraryVersion
	if e = json.Unmarshal(data, &v); e != nil {
		return e
	}
	return p.open(ctx, project, func(w *workspace.Session) error {
		for path, hash := range v.Files {
			data, e := p.get(hash)
			if e != nil {
				return e
			}
			dest, e := workspace.SafePath(w.Root, path)
			if e != nil {
				return e
			}
			if prior, e := os.ReadFile(dest); e == nil && digest(prior) != hash {
				return fmt.Errorf("library asset path collision: %s", path)
			} else if e != nil && !os.IsNotExist(e) {
				return e
			}
			if e = os.MkdirAll(filepath.Dir(dest), 0700); e != nil {
				return e
			}
			if e = os.WriteFile(dest, data, 0600); e != nil {
				return e
			}
		}
		return fn(w, v)
	}, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `INSERT INTO pt_bindings VALUES($1,$2,$3,$4) ON CONFLICT(project_id,package_path) DO UPDATE SET id=excluded.id,version=excluded.version`, project, v.Package, id, version)
		return e
	})
}
func (p *Postgres) DeleteLibrary(ctx context.Context, id, version string) error {
	c, e := p.readyConnect(ctx)
	if e != nil {
		return e
	}
	defer func() { _ = c.Close(context.Background()) }()
	_, e = c.Exec(ctx, `DELETE FROM pt_library WHERE id=$1 AND version=$2`, id, version)
	if e != nil {
		return fmt.Errorf("library version is referenced or deletion failed")
	}
	return nil
}
