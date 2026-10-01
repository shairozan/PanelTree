# ADR 002: Alternate PostgreSQL authoring storage

Status: implementation decision for #25. Filesystem remains supported.

PostgreSQL is authoritative for `pg:<project-id>` handles. Host configuration selects
an environment variable containing the DSN and an absolute blob-store directory.
No DSN or credentials occur in project records or portable exports.

Projects have relational identity and revision counters. Ordered document records
store validated typed `model.Document` JSONB (not YAML text); nested panel/layer
arrays preserve the existing strict schema. Editorial and reference manifests are
separate JSONB records. Asset rows map project-relative paths to immutable global
SHA-256 blobs. The shared library owns immutable character/version manifests and
project bindings use foreign keys. Binary artwork remains outside PostgreSQL.

Existing loaders/renderers need relative files. The adapter materializes a private
operation workspace from typed records and verified blobs, invokes shared policy,
validates/captures the resulting snapshot and commits metadata transactionally.
The materialization is disposable, never a second authoritative checkout. Content
revisions stay based on the same canonical project/state algorithm as file storage.
PostgreSQL advisory locks serialize project operations; database transactions and
revision checks prevent partial publication. Blob staging precedes metadata commit;
unreferenced staged blobs are retained (automatic GC is deliberately absent).

Jobs use a separate persistence boundary with PostgreSQL state and frozen payloads,
not delayed workspace export. Submission markers and remote IDs must commit before
subsequent network calls. Executor ownership and metadata serialization live in
PostgreSQL; recovery preserves known remote executions and refuses ambiguous retries.

Import/export is explicit, bounded and path-validated. Portable bundles contain
story documents, state, reference lineage, job history and required blobs. Imported
unfinished jobs are held/cancelled and cannot run merely because a worker starts.
Runtime settings outside the story assets, managed caches and locks are excluded. Restore requires
both the database and the blob directory. Library upgrades/forks are explicit and
must pass the same project lock/revision policy as edits.

This is a trusted single-operator deployment, not multi-tenant authorization.
PostgreSQL session locks require direct/session-pooled connections (not transaction
pooling). Rendering remains a leaf contract. Frontend work belongs to #26.
