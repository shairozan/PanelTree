# Revision-aware editing and artwork protection

Use `inspect project.yaml` to obtain the current `revision`. Pass that value with
every edit. CLI commands and `app.Service.Edit` share the same transaction and
policy checks. A stale revision fails; inspect again and reconcile the changes.
Editorial state participates in the revision, so an approval or lock also makes
an earlier edit request stale. Direct changes to authoring YAML change the next
revision, but external editors do not participate in transaction serialization.

## Commands

All layer operations identify `--page`, `--panel`, and `--layer`, plus the
`--revision` from the preceding result. Layer IDs are scoped to a panel.

```sh
paneltree inspect my-book/project.yaml
paneltree review my-book/project.yaml --revision REVISION --page page-01 --panel p1 --layer hero
paneltree approve my-book/project.yaml --revision NEW_REVISION --page page-01 --panel p1 --layer hero --artifact assets/hero.png
paneltree lock my-book/project.yaml --revision NEW_REVISION --page page-01 --panel p1 --layer hero
paneltree unlock my-book/project.yaml --revision NEW_REVISION --page page-01 --panel p1 --layer hero
```

`draft` explicitly returns a reviewed/approved layer to draft without losing its
selection. `review` requires draft; `approve` requires review and an existing PNG.
Approval copies and hashes the PNG into `.paneltree/assets/`, outside the cache.
Deleting `.paneltree/cache/` cannot remove approved artwork. Keep `.paneltree/`
with the project when backing it up; it is ignored by Git by default.

`override --artifact PATH` selects a manual PNG and enters draft. The original
file remains externally editable and is never written by the service. Relative
artifact paths resolve from the project root; absolute paths are accepted. Manual
paths are stored as canonical absolute paths, so relocating a project with manual
overrides requires selecting their new locations. Approval freezes a copy.
`clear-selection` explicitly returns to the authoring source and draft state.

Changing a source request, draft revision/seed, or source/font bytes retains the
selected artwork and reports `stale: true` in the layer status returned by edits,
inspection, and builds. Staleness currently describes source provenance; output
resolution and placement are independent composition choices. Missing/corrupt
pins fail before rasterization, including on warm-cache builds. They are never
regenerated. Restore the protected file or explicitly clear/reselect it.

## Lock scopes

`lock` defaults to `--scope all`. `asset` protects source requests, source/font
contents and selections, including selected manual artwork below a group.
`placement` protects layer/group geometry, stacking order, ancestor geometry,
page layout/canvas/background, and mask paths/contents. Artwork requests and
selection can still change under a placement-only lock. `all` combines both.

Locks protect deletion and ancestor changes, not just direct edits to the target.
Placement comparisons are conservative: changing a page layout can require
unlocking a layer even if its resulting pixel bounds would remain unchanged.
An existing lock cannot be weakened with another `lock` command. First commit
an explicit `unlock`, then use its returned revision for the protected edit.
Unlock-only transactions can acknowledge externally changed protected files.
Source/mask/manual-file changes outside the service are detected on subsequent
operations; locks cannot stop another program from modifying files on disk.

## Typed multi-file changesets

`paneltree edit my-book/project.yaml --changes change.json` accepts:

```json
{
  "expected_revision": "REVISION_FROM_INSPECT",
  "edits": [
    {
      "file": "project.yaml",
      "document": {
        "schema": "paneltree/v0.1",
        "book": {
          "id": "my-book",
          "title": "Revised title",
          "chapters": ["chapters/01.yaml"]
        }
      }
    }
  ],
  "operations": [
    {"target": {"page": "page-01", "panel": "p1", "layer": "hero"}, "action": "review"}
  ]
}
```

Each edit replaces one existing authoring document with a typed document, using
the same fields as project YAML. Preserve its ID and references unless changing
them intentionally. The complete staged document graph passes strict schema and
editorial-policy validation before publication. Unknown fields and duplicate
document edits fail. Paths must stay within the workspace and writable document
paths cannot traverse symlinks. This first editing API supports replacement of
existing documents and removal of references, not creation of new document files.
Orphaned authoring files are retained on disk. Use the owning `project.yaml` for
edits to an initialized book; builds/inspection still accept nested input files.

Operations use the command names above as `action`, with optional `scope` and
`artifact`. There is at most one operation per layer per changeset. An unlock
cannot bypass a protected edit in the same changeset. Clear a durable selection
before deleting its layer.

## Serialization and recovery

An OS-backed project lock serializes participating reads, builds and writes across
processes; process exit releases the lock. Cancellation while waiting does not
acquire it. Builds currently hold this lock through export, so edits wait for an
active build. State and changed documents are staged before a flushed journal is
published. After the journal exists, recovery rolls forward all files before any
service observes the project. Cancellation before that commit point preserves
the old revision; after it, publication completes even if cancellation arrives.

An interrupted replay leaves the journal for the next operation to finish. Do not
manually remove `.paneltree/pending.json` after an interruption. This guarantee
covers process interruption on a local filesystem. Sudden power loss, filesystem
corruption, hostile metadata modification, network filesystems, and concurrent
external editors are not covered by the tested recovery contract. Raw callers of
`internal/project.Load` do not acquire the workspace lock; interfaces should use
the shared application services.
