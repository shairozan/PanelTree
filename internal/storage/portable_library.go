package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
)

func (p *Postgres) projectLibrary(ctx context.Context, c *pgx.Conn, id string) ([]LibraryVersion, error) {
	rows, e := c.Query(ctx, `SELECT DISTINCT l.id,l.version,l.payload FROM pt_bindings b JOIN pt_library l ON b.id=l.id AND b.version=l.version WHERE b.project_id=$1 ORDER BY l.id,l.version`, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []LibraryVersion
	for rows.Next() {
		var v LibraryVersion
		var data []byte
		var id, version string
		if e = rows.Scan(&id, &version, &data); e != nil {
			return nil, e
		}
		if e = json.Unmarshal(data, &v); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (p *Postgres) restoreLibrary(ctx context.Context, tx pgx.Tx, project string, versions []LibraryVersion, files map[string][]byte) error {
	if len(versions) > 1024 {
		return fmt.Errorf("too many library versions")
	}
	for _, v := range versions {
		if !identifier.MatchString(v.ID) || !identifier.MatchString(v.Version) || len(v.Files) == 0 {
			return fmt.Errorf("invalid portable library version")
		}
		if _, ok := v.Files[v.Package]; !ok {
			return fmt.Errorf("library package missing")
		}
		for path, hash := range v.Files {
			if b, ok := files[path]; !ok || digest(b) != hash {
				return fmt.Errorf("library backup missing verified asset %s", path)
			}
		}
		var existing []byte
		e := tx.QueryRow(ctx, `SELECT payload FROM pt_library WHERE id=$1 AND version=$2`, v.ID, v.Version).Scan(&existing)
		data, e2 := json.Marshal(v)
		if e2 != nil {
			return e2
		}
		switch e {
		case nil:
			var other LibraryVersion
			if e = json.Unmarshal(existing, &other); e != nil {
				return e
			}
			otherData, _ := json.Marshal(other)
			if string(data) != string(otherData) {
				return fmt.Errorf("library version collision")
			}
		case pgx.ErrNoRows:
			if _, e = tx.Exec(ctx, `INSERT INTO pt_library VALUES($1,$2,$3)`, v.ID, v.Version, data); e != nil {
				return e
			}
			for path, hash := range v.Files {
				b, ok := files[path]
				if !ok || digest(b) != hash {
					return fmt.Errorf("library backup missing verified asset %s", path)
				}
				if _, e = p.put(b); e != nil {
					return e
				}
				if _, e = tx.Exec(ctx, `INSERT INTO pt_blobs VALUES($1,$2) ON CONFLICT DO NOTHING`, hash, len(b)); e != nil {
					return e
				}
				if _, e = tx.Exec(ctx, `INSERT INTO pt_library_assets VALUES($1,$2,$3,$4)`, v.ID, v.Version, path, hash); e != nil {
					return e
				}
			}
		default:
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO pt_bindings VALUES($1,$2,$3,$4) ON CONFLICT(project_id,package_path) DO UPDATE SET id=excluded.id,version=excluded.version`, project, v.Package, v.ID, v.Version); e != nil {
			return e
		}
	}
	return nil
}
