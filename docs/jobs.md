# Durable asset jobs

Asset jobs render one authored leaf into a separate PNG candidate. Submission and
completion never select artwork or alter source assets. CLI commands use the same
application services available to future interfaces.

## Workflow

Run `paneltree renderers` to discover capabilities. The `builtin` renderer supports
PNG image, SVG and basic-Latin text sources under the existing renderer limits.
`comfyui` is advertised as unavailable; requesting it or an unknown renderer
returns a typed `app.JobDiagnostic` with code `renderer_unavailable`. The CLI
prints diagnostic errors as `code: message`; successful command results are JSON.

Use the owning project file and its revision from `paneltree inspect`. Replace
`REVISION` and `JOB_ID` below with returned values:

```sh
paneltree asset request my-book/project.yaml --revision REVISION --key hero-draft-1 --page page-01 --panel p1 --layer hero
paneltree jobs list my-book/project.yaml
paneltree jobs run my-book/project.yaml --workers 2
paneltree jobs status my-book/project.yaml --id JOB_ID
paneltree asset select my-book/project.yaml --revision REVISION --id JOB_ID
```

Request output dimensions describe the page, as for builds: optional `--width`,
`--height` and `--fit` determine the resolved leaf geometry. Submission freezes
the project revision, leaf request, renderer version, source/font bytes and
effective-input fingerprint. Page dependencies must be readable to resolve layout.
Workers use the frozen bytes even if project files change afterward.

An identical retry with the same key, original revision, target and output options
returns the original job, including after later edits. A different request under
that key fails with `idempotency_conflict`. Keys identify jobs across terminal
states too; use a new key for a deliberate retry after failure or cancellation.

## Execution and cancellation

`jobs run` drains queued work in the foreground with 1–8 workers (default 2).
It does not install a daemon. Only one executor may own a project's queue; a
second concurrent runner receives `runner_busy`. Jobs are claimed in job-ID
order, with no FIFO or fairness guarantee. Rendering does not hold the project
editing lock.

States are `queued`, `running`, `succeeded`, `failed` and `cancelled`. Status
includes the original revision, progress, diagnostic and integrity hashes.
Progress reaches 100 only after a complete artifact is persisted. A job failure
is recorded on that job; a successful queue-drain command does not mean every
job succeeded, so inspect its returned statuses.

```sh
paneltree jobs cancel my-book/project.yaml --id JOB_ID
```

Queued cancellation is immediate. Running cancellation requests cooperative
interruption; completion checks cancellation before publishing success. Failed
and cancelled jobs expose no successful result, even when a renderer produced
partial bytes. Cancellation of an already terminal job leaves its result intact.

## Candidate selection and provenance

Selection is explicit and requires both the current expected project revision
and the job's original revision to agree. It also recomputes the effective-input
fingerprint, rejecting source/font changes even without a project revision change.
Stale candidates remain recorded for inspection but cannot be selected.

Selection follows editorial lock policy and refuses to replace approved artwork
or a manual override. Clear those selections explicitly before submitting a new
request at the resulting revision. Successful selection copies the candidate to
the durable asset store and marks it draft; approval remains a separate operation.
Authoring sources remain unchanged. Existing selected artwork is never replaced
merely because a job completes.

## Storage and recovery

The owning project stores jobs beneath `.paneltree/jobs/<job-id>/`. `input.json`
contains frozen provenance and dependencies; numbered JSON snapshots record
state, and `artifact` contains the successful PNG. Public result retrieval checks
both input and artifact hashes. Back up the job store when this history matters,
and keep `.paneltree/assets` with the project's editorial state for selected art.

OS-backed metadata and executor locks coordinate participating local processes.
State snapshots are published atomically. If an executor dies, its OS lock is
released; the next store operation recovers abandoned running jobs as `failed`
with diagnostic `interrupted` (or `cancelled` when cancellation was pending).
Queued work remains queued for the next runner. Interrupted work is not
automatically retried. An orphan artifact from interrupted publication is never
treated as a successful candidate.

This supports local process interruption, not power-loss durability, network
filesystems, distributed scheduling, concurrent external modification of store
files, or automatic garbage collection. Source snapshotting does not coordinate
external editors. Frozen requests require the recorded renderer/runtime version;
a different version fails safely. ComfyUI transport is outside Sprint 07.
