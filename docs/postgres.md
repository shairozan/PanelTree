# PostgreSQL storage and shared characters

PostgreSQL is optional. Existing project.yaml paths still use the filesystem.
Database projects use `pg:<id>` in the same inspect, edit, build, job and character
reference commands. CLI, MCP and the local browser workspace call shared application services.
This is a trusted local/single-operator deployment, not a multi-tenant server.

## Configure and initialize

For local development, run PostgreSQL 18 with a persistent database volume. For
example, in PowerShell (choose a local password before running):

```powershell
$env:POSTGRES_PASSWORD = 'choose-a-local-password'
docker run -d --name paneltree-db -p 127.0.0.1:5432:5432 -e POSTGRES_PASSWORD -e POSTGRES_DB=paneltree -v paneltree-db:/var/lib/postgresql postgres:18
$env:PANELTREE_DATABASE_URL = "postgres://postgres:$([Uri]::EscapeDataString($env:POSTGRES_PASSWORD))@localhost:5432/paneltree?sslmode=disable"
```

Set `PANELTREE_DATABASE_URL` to your PostgreSQL connection URL in the host environment.
Keep passwords/API keys out of story YAML, source control and exported projects.
Use a direct or session-pooled connection; transaction pooling cannot preserve the
session locks used by workspace and worker ownership.

Example runtime YAML (the DSN variable's name, not its value):

```yaml
storage:
  dsn-env: PANELTREE_DATABASE_URL
  blob-root: ./paneltree-data
```

The blob root is relative to this runtime file. All processes using a database must
share the same durable blob directory. Use a separate blob root per database or
schema. The adapter stores database metadata and global content-addressed originals
there, with disposable rendering scratch files kept separately.

```text
paneltree storage migrate --config runtime.yaml
paneltree init pg:my-story --config runtime.yaml
paneltree storage import existing/project.yaml --id imported-story --config runtime.yaml
paneltree storage list --config runtime.yaml
paneltree inspect pg:my-story --config runtime.yaml
paneltree build pg:my-story --page page-01 --output page.png --config runtime.yaml
paneltree storage export pg:my-story --output portable-story --config runtime.yaml
```

Migrations are explicit and transactional. Unknown newer schema versions are
rejected. Import and export require a new ID/directory and never overwrite an
existing project. Imported projects are independent copies; no automatic YAML/DB
synchronization takes place. Runtime configuration is not imported.

## Shared character library

Publish a character package from either a file or database project. Optionally
include an already-published multi-view reference set:

```text
paneltree character-library --action publish --project project.yaml --id patrick --version design-1 --package characters/patrick.json --set patrick --reference-version design-1 --config runtime.yaml
paneltree character-library --action list --config runtime.yaml
paneltree character-library --action use --project pg:my-story --id patrick --version design-1 --revision CURRENT --page page-01 --panel p1 --layer hero --config runtime.yaml
```

Omit set/reference-version for a package without a published image set. Library
versions are immutable. Publishing design-2 leaves existing bindings intact;
explicit `use` selects the new version subject to current project revisions and
locks. Project costume/expression/pose/prop selections remain independent.
Publish from a project with a different library ID to fork its character design.
To change the package's own descriptions, edit its source package before publishing.
Canonical original blobs are deduplicated across projects; operation workspaces
materialize copies for the existing renderers, not new canonical artwork.

Deletion (`--action delete --id ... --version ...`) is rejected while project
bindings reference the version. Older bindings are retained conservatively even
after a panel moves to a newer version, preserving provenance and preventing
accidental asset removal. No garbage collector is implemented.

## Durability and portable backups

Project edits/reference approvals commit atomically. Jobs persist separately so
submission markers, remote IDs and checkpoints commit before network operations.
Database executor locks and ownership tokens prevent a superseded runner from
publishing over a newer owner. Recovery resumes known remote executions; ambiguous
submissions are never automatically retried.

Portable project exports include typed authoring content, editorial state,
reference manifests, required originals, shared library version manifests/bindings
and job history. Unfinished jobs are held as cancelled in imported/exported copies.
Manual overrides are snapshotted into portable immutable assets. Their old filename
remains provenance for legacy selections, but a database selection no longer follows
later edits to that file; explicitly override again to select updated bytes. Exported
projects retain these snapshots even after the source directory is moved. New database
selections use an immutable asset path, so overriding one layer cannot change a sibling.

A known remote execution may be explicitly resumed; otherwise submit a new request
only after checking the provider's billing dashboard. Importing data never renders.

For full deployment backup, stop writers/workers, use PostgreSQL `pg_dump` for the
database, and copy the blob directory from the same quiescent state. Restore the
DB and blobs together before restarting services. Keep host secrets/configuration
in a separate secure backup. Do not back up only disposable scratch/cache files.
Per-project `storage export`/`storage import` is also a portable logical backup;
tests restore it into a fresh PostgreSQL schema and separate blob directory.

MCP accepts pg handles in existing project tools and exposes `storage_manage` and
`character_library`. Explicit storage configuration grants that trusted MCP host
access to the configured database projects. Filesystem imports and artifact
outputs still require configured MCP roots. Credentials are server-side only.

## Implementation and validation

See [ADR 002](adr/002-postgres-storage.md). Typed documents use JSONB records with
preserved ordering; file blobs, editorial/reference manifests and library bindings
have separate tables. Temporary workspaces bridge existing file-based renderers.

Set `PANELTREE_TEST_POSTGRES` to a disposable PostgreSQL URL to enable integration
tests. The CI PostgreSQL service runs the complete suite and affected race tests
on every push/PR; ordinary filesystem tests do not require a database. Integration
tests create uniquely named records and temporary schemas, so never point them at
production. No image-generation API calls are made by these tests.

File-backed stories can also use a shared published version. The service copies
immutable assets and a portable library manifest, so a later database import
retains the version bindings. The [browser workspace](web-workspace.md) provides
publication, reference review and explicit pinned-version selection.
