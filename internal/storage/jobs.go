package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/shairozan/PanelTree/internal/jobs"
	"path/filepath"
	"sync/atomic"
	"time"
)

func migrateJobs(ctx context.Context, tx pgx.Tx) error {
	_, e := tx.Exec(ctx, `CREATE TABLE pt_jobs(project_id text REFERENCES pt_projects(id),id text,sequence integer NOT NULL,payload jsonb NOT NULL,input bytea NOT NULL,artifact text REFERENCES pt_blobs(hash),PRIMARY KEY(project_id,id)); CREATE TABLE pt_job_owners(project_id text PRIMARY KEY REFERENCES pt_projects(id),token bigint NOT NULL);UPDATE pt_schema SET version=2;`)
	return e
}

type jobPersistence struct {
	p     *Postgres
	id    string
	token atomic.Int64
	pid   atomic.Int64
}

func (p *Postgres) Jobs(id string) (*jobs.Store, error) {
	if !identifier.MatchString(id) {
		return nil, fmt.Errorf("invalid project ID")
	}
	return jobs.NewPersistent(filepath.Join(p.root, "scratch", id), &jobPersistence{p: p, id: id})
}
func (j *jobPersistence) with(fn func(context.Context, *pgx.Conn) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, e := j.p.readyConnect(ctx)
	if e != nil {
		return e
	}
	defer func() { _ = c.Close(context.Background()) }()
	if token := j.token.Load(); token != 0 {
		var owned bool
		e = c.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pt_job_owners WHERE project_id=$1 AND token=$2) AND EXISTS(SELECT 1 FROM pg_locks WHERE pid=$3 AND locktype='advisory' AND granted AND classid=((hashtextextended($4,26)>>32)&4294967295)::oid AND objid=(hashtextextended($4,26)&4294967295)::oid AND objsubid=1)`, j.id, token, j.pid.Load(), j.id+"/executor.lock").Scan(&owned)
		if e != nil {
			return e
		}
		if !owned {
			return fmt.Errorf("executor ownership lost")
		}
	}
	return fn(ctx, c)
}
func (j *jobPersistence) Read(id string) (jobs.Job, error) {
	var out jobs.Job
	e := j.with(func(ctx context.Context, c *pgx.Conn) error {
		var b []byte
		if e := c.QueryRow(ctx, `SELECT payload FROM pt_jobs WHERE project_id=$1 AND id=$2`, j.id, id).Scan(&b); e != nil {
			if e == pgx.ErrNoRows {
				return &jobs.Diagnostic{Code: "not_found", Message: "job does not exist"}
			}
			return e
		}
		return json.Unmarshal(b, &out)
	})
	return out, e
}
func (j *jobPersistence) Write(v jobs.Job) error {
	return j.with(func(ctx context.Context, c *pgx.Conn) error {
		b, e := json.Marshal(v)
		if e != nil {
			return e
		}
		tag, e := c.Exec(ctx, `UPDATE pt_jobs SET sequence=$3,payload=$4 WHERE project_id=$1 AND id=$2 AND sequence=$3-1 AND ($5::bigint=0 OR EXISTS(SELECT 1 FROM pt_job_owners WHERE project_id=$1 AND token=$5))`, j.id, v.ID, v.Sequence, b, j.token.Load())
		if e == nil && tag.RowsAffected() != 1 {
			return fmt.Errorf("job revision or executor ownership conflict")
		}
		return e
	})
}
func (j *jobPersistence) List() ([]jobs.Job, error) {
	var out []jobs.Job
	e := j.with(func(ctx context.Context, c *pgx.Conn) error {
		rows, e := c.Query(ctx, `SELECT payload FROM pt_jobs WHERE project_id=$1 ORDER BY id`, j.id)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var b []byte
			var v jobs.Job
			if e = rows.Scan(&b); e != nil {
				return e
			}
			if e = json.Unmarshal(b, &v); e != nil {
				return e
			}
			out = append(out, v)
		}
		return rows.Err()
	})
	return out, e
}
func (j *jobPersistence) Submit(v jobs.Job, input json.RawMessage) error {
	return j.with(func(ctx context.Context, c *pgx.Conn) error {
		b, e := json.Marshal(v)
		if e != nil {
			return e
		}
		_, e = c.Exec(ctx, `INSERT INTO pt_jobs(project_id,id,sequence,payload,input) VALUES($1,$2,$3,$4,$5)`, j.id, v.ID, v.Sequence, b, []byte(input))
		return e
	})
}
func (j *jobPersistence) Bytes(id, kind string) ([]byte, error) {
	var data []byte
	e := j.with(func(ctx context.Context, c *pgx.Conn) error {
		if kind == "input" {
			return c.QueryRow(ctx, `SELECT input FROM pt_jobs WHERE project_id=$1 AND id=$2`, j.id, id).Scan(&data)
		}
		var hash string
		if e := c.QueryRow(ctx, `SELECT artifact FROM pt_jobs WHERE project_id=$1 AND id=$2`, j.id, id).Scan(&hash); e != nil {
			return e
		}
		var e error
		data, e = j.p.get(hash)
		return e
	})
	return data, e
}
func (j *jobPersistence) PutArtifact(id string, data []byte) error {
	return j.with(func(ctx context.Context, c *pgx.Conn) error {
		hash, e := j.p.put(data)
		if e != nil {
			return e
		}
		tx, e := c.Begin(ctx)
		if e != nil {
			return e
		}
		defer func() { _ = tx.Rollback(context.Background()) }()
		if _, e = tx.Exec(ctx, `INSERT INTO pt_blobs VALUES($1,$2) ON CONFLICT DO NOTHING`, hash, len(data)); e != nil {
			return e
		}
		tag, e := tx.Exec(ctx, `UPDATE pt_jobs SET artifact=$3 WHERE project_id=$1 AND id=$2 AND EXISTS(SELECT 1 FROM pt_job_owners WHERE project_id=$1 AND token=$4)`, j.id, id, hash, j.token.Load())
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return fmt.Errorf("executor ownership lost")
		}
		return tx.Commit(ctx)
	})
}
func (j *jobPersistence) Lock(ctx context.Context, name string, wait bool) (func(), error) {
	// with() probes for abandoned execution on every metadata operation. An
	// existing worker must never use that probe to acquire a fresh fence after
	// its original session was lost.
	if name == "executor.lock" && j.token.Load() != 0 {
		return nil, &jobs.Diagnostic{Code: "runner_busy", Message: "execution already owned by this worker"}
	}
	c, e := j.p.readyConnect(ctx)
	if e != nil {
		return nil, e
	}
	fail := func(err error) (func(), error) { _ = c.Close(context.Background()); return nil, err }
	for {
		var acquired bool
		if e = c.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtextextended($1,26))`, j.id+"/"+name).Scan(&acquired); e != nil {
			return fail(e)
		}
		if acquired {
			break
		}
		if !wait {
			return fail(&jobs.Diagnostic{Code: "runner_busy", Message: "another process owns execution"})
		}
		select {
		case <-ctx.Done():
			return fail(ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
	var token int64
	if name == "executor.lock" {
		if e = c.QueryRow(ctx, `INSERT INTO pt_job_owners VALUES($1,1) ON CONFLICT(project_id) DO UPDATE SET token=pt_job_owners.token+1 RETURNING token`, j.id).Scan(&token); e != nil {
			return fail(e)
		}
		j.pid.Store(int64(c.PgConn().PID()))
		j.token.Store(token)
	}
	return func() {
		_ = c.Close(context.Background())
		if token != 0 {
			j.token.CompareAndSwap(token, 0)
		}
	}, nil
}
