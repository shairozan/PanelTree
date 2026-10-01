package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/shairozan/PanelTree/internal/jobs"
	"github.com/shairozan/PanelTree/internal/workspace"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
)

type portableJob struct {
	Job      jobs.Job
	Input    json.RawMessage
	Artifact []byte
}

var jobID = regexp.MustCompile(`^[a-f0-9]{64}$`)

func captureJobs(root string) ([]portableJob, error) {
	dir := filepath.Join(root, ".paneltree/jobs")
	entries, e := os.ReadDir(dir)
	if os.IsNotExist(e) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	var out []portableJob
	for _, entry := range entries {
		if !jobID.MatchString(entry.Name()) {
			continue
		}
		base, e := workspace.SafePath(root, ".paneltree/jobs/"+entry.Name())
		if e != nil {
			return nil, e
		}
		states, e := os.ReadDir(base)
		if e != nil {
			return nil, e
		}
		var names []string
		for _, st := range states {
			if regexp.MustCompile(`^[0-9]{8}\.json$`).MatchString(st.Name()) {
				names = append(names, st.Name())
			}
		}
		sort.Strings(names)
		if len(names) == 0 {
			return nil, fmt.Errorf("job history missing")
		}
		data, e := readPortableFile(base, names[len(names)-1], 4<<20)
		if e != nil {
			return nil, e
		}
		var v portableJob
		if e = json.Unmarshal(data, &v.Job); e != nil {
			return nil, e
		}
		v.Input, e = readPortableFile(base, "input.json", 128<<20)
		if e != nil {
			return nil, e
		}
		if v.Job.State == jobs.Succeeded {
			v.Artifact, e = readPortableFile(base, "artifact", 32<<20)
			if e != nil {
				return nil, e
			}
		}
		if e = validPortableJob(v); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, nil
}
func validPortableJob(v portableJob) error {
	if v.Job.ID != digest([]byte(v.Job.Key)) || v.Job.InputHash != digest(v.Input) || !json.Valid(v.Input) || len(v.Input) > 128<<20 || v.Job.Sequence < 1 {
		return fmt.Errorf("invalid portable job")
	}
	if v.Job.State == jobs.Succeeded && (len(v.Artifact) > 32<<20 || digest(v.Artifact) != v.Job.ArtifactHash) {
		return fmt.Errorf("invalid portable job artifact")
	}
	return nil
}
func held(v portableJob) portableJob {
	if v.Job.State == jobs.Queued || v.Job.State == jobs.Running {
		v.Job.State = jobs.Cancelled
		v.Job.Sequence++
		v.Job.Diagnostic = &jobs.Diagnostic{Code: "imported_hold", Message: "imported unfinished work; explicitly resume a known remote execution or submit a new request"}
	}
	return v
}
func writePortableJobs(root string, all []portableJob) error {
	for _, v := range all {
		if e := validPortableJob(v); e != nil {
			return e
		}
		v = held(v)
		dir, e := workspace.SafePath(root, ".paneltree/jobs/"+v.Job.ID)
		if e != nil {
			return e
		}
		if e = os.MkdirAll(dir, 0700); e != nil {
			return e
		}
		data, e := json.Marshal(v.Job)
		if e != nil {
			return e
		}
		for name, b := range map[string][]byte{fmt.Sprintf("%08d.json", v.Job.Sequence): data, "input.json": v.Input} {
			if e = os.WriteFile(filepath.Join(dir, name), b, 0600); e != nil {
				return e
			}
		}
		if len(v.Artifact) > 0 {
			if e = os.WriteFile(filepath.Join(dir, "artifact"), v.Artifact, 0600); e != nil {
				return e
			}
		}
	}
	return nil
}
func (p *Postgres) importJobs(ctx context.Context, tx pgx.Tx, id string, all []portableJob) error {
	for _, v := range all {
		if e := validPortableJob(v); e != nil {
			return e
		}
		v = held(v)
		var artifact *string
		if len(v.Artifact) > 0 {
			hash, e := p.put(v.Artifact)
			if e != nil {
				return e
			}
			if _, e = tx.Exec(ctx, `INSERT INTO pt_blobs VALUES($1,$2) ON CONFLICT DO NOTHING`, hash, len(v.Artifact)); e != nil {
				return e
			}
			artifact = &hash
		}
		data, e := json.Marshal(v.Job)
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO pt_jobs VALUES($1,$2,$3,$4,$5,$6)`, id, v.Job.ID, v.Job.Sequence, data, []byte(v.Input), artifact); e != nil {
			return e
		}
	}
	return nil
}
func (p *Postgres) exportJobs(ctx context.Context, id string) ([]portableJob, error) {
	c, e := p.readyConnect(ctx)
	if e != nil {
		return nil, e
	}
	defer func() { _ = c.Close(context.Background()) }()
	rows, e := c.Query(ctx, `SELECT payload,input,artifact FROM pt_jobs WHERE project_id=$1 ORDER BY id`, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []portableJob
	for rows.Next() {
		var v portableJob
		var data []byte
		var artifact *string
		if e = rows.Scan(&data, &v.Input, &artifact); e != nil {
			return nil, e
		}
		if e = json.Unmarshal(data, &v.Job); e != nil {
			return nil, e
		}
		if v.Job.State == jobs.Succeeded && artifact != nil {
			v.Artifact, e = p.get(*artifact)
			if e != nil {
				return nil, e
			}
		}
		if e = validPortableJob(v); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// Bound reads before allocating and reject links, including paths inside job directories.
func readPortableFile(root, rel string, limit int64) ([]byte, error) {
	path, e := workspace.SafePath(root, rel)
	if e != nil {
		return nil, e
	}
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer func() { _ = f.Close() }()
	st, e := f.Stat()
	if e != nil {
		return nil, e
	}
	if !st.Mode().IsRegular() || st.Size() > limit {
		return nil, fmt.Errorf("portable file exceeds limit: %s", rel)
	}
	b, e := io.ReadAll(io.LimitReader(f, limit+1))
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("portable file exceeds limit: %s", rel)
	}
	return b, e
}
