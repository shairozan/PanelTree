# Local MCP authoring

`paneltree mcp serve` exposes shared application services over newline-delimited
JSON-RPC on stdin/stdout, using the official Go MCP SDK pinned in `go.mod`.
There is no HTTP listener, subprocess command wrapper, hosted service, or model
embedded in the server. Standard output contains only protocol messages; startup
errors go to stderr. Closing client stdin ends the session.

## Configure a client

Build the binary and supply at least one existing allowed directory:

```sh
go build -o bin/paneltree ./cmd/paneltree
bin/paneltree mcp serve --root /home/artist/books
```

On Windows use `bin/paneltree.exe` and an absolute Windows root. For clients with
an `mcpServers` configuration, an example is:

```json
{
  "mcpServers": {
    "paneltree": {
      "command": "C:/tools/paneltree.exe",
      "args": ["mcp", "serve", "--root", "C:/Users/artist/Books"]
    }
  }
}
```

Repeat `--root` for multiple roots. Alternatively, put `mcp-roots` in an explicit
runtime file and launch with `--config runtime.yaml`:

```yaml
log-level: info
mcp-roots:
  - C:/Users/artist/Books
```

Explicit `--root` values replace the file's root list. Each command invocation
uses its own Viper instance and typed configuration. The roots are server-owned;
client roots notifications cannot expand access. No renderer credentials are
needed or exposed by this implementation. Future credentials belong in runtime
configuration, never book YAML or MCP tool arguments. The server does not publish
its runtime configuration as a resource.

## Tools and authoring sequence

Every tool has an input and output JSON Schema. Successful tool results include
`structuredContent.data` and a JSON text representation. Application failures
set `isError: true` and return `structuredContent.error` with `code` and `message`.
Codes include `outside_root`, `recovery_required`, job diagnostic codes such as
`renderer_unavailable`, and `operation_failed` for other shared-service failures.
Malformed protocol requests and schema-invalid arguments use MCP protocol errors.
Treat either error channel as failure; never infer success from transport success.

| Tool | Purpose / required arguments |
| --- | --- |
| `project_list` | Discover `project.yaml` files beneath configured roots; no arguments |
| `project_init` | Create a new book at absolute `directory`; existing destinations are preserved |
| `project_open`, `project_inspect` | Read documents, IDs, revision, editorial states and resolved scenes from `project_file` |
| `project_validate` | Validate `project_file` through shared policy |
| `project_apply_changes` | Submit `project_file`, `expected_revision`, and optional `edits` / `operations` |
| `renderer_list` | Discover available static renderers and unavailable ComfyUI capability |
| `asset_request` | Submit `project_file`, `expected_revision`, `key`, and `target`; optional renderer/output dimensions |
| `asset_select` | Select a successful job with `project_file`, `id`, and `expected_revision` |
| `build` | Export a page with `project_file`, `page`, and exactly one of `output` / `bundle` |
| `jobs_list` | List jobs for `project_file` |
| `jobs_status`, `jobs_cancel` | Poll or cancel `id` in `project_file` |
| `jobs_run` | Drain the queue for `project_file`; `workers` defaults to 2 (1–8) |

All project operations require the owning file named `project.yaml`. Path arguments
are absolute, except document edit `file` paths, which are relative to that owner.
Use `project_open` to obtain current documents and IDs before editing. For example:

```json
{
  "project_file": "C:/Users/artist/Books/my-book/project.yaml",
  "expected_revision": "REVISION_FROM_OPEN",
  "operations": [
    {"target": {"page": "page-01", "panel": "p1", "layer": "hero"},
     "action": "review"}
  ]
}
```

The same changeset tool handles `draft`, `review`, `approve`, `override`,
`clear-selection`, `lock`, and `unlock`. `scope` is `asset`, `placement`, or `all`
for locks. `artifact` supplies the absolute PNG path for approval or override
when needed. Document edits contain the full typed `document`, not YAML text.
See [editing](editing.md) for shared transaction rules. Reopen after edits to use
the resulting revision. A stale edit or lock bypass fails through the same policy
as the CLI; MCP does not unlock or overwrite selections automatically.

For a candidate, submit `asset_request` with the current revision, unique key and
target, call `jobs_run`, then poll `jobs_status`. Request completion leaves artwork
unchanged. `asset_select` explicitly selects a successful candidate and rejects
stale revisions/dependencies. See [jobs](jobs.md) for durable states and recovery.

`build` accepts optional `width`, `height`, `fit`, and `no_cache`. The `output`
destination must be a new PNG file with an existing parent directory. The `bundle`
destination is an existing directory beneath which the shared exporter publishes
a new portable bundle with PNG, SVG and editable sources. PDF is not available.

## Resources and previews

`paneltree://workflow` is always readable. Opening or initializing a book registers
a `paneltree://project/<id>` resource containing the current inspection as JSON.
Find it via `resources/list`. Its stable ID derives from the canonical project
path; the resource is live, so read its revision before changes.

Build results contain `build` (the shared build result), `preview` (the PNG
resource URI), and `artifacts` (all exported resource URIs). Call `resources/read`
with those URIs. PNGs are returned as `image/png` blobs encoded by MCP for client
display; bundle SVGs use `image/svg+xml`. Artifact IDs incorporate path and content
hash. Reads recheck containment and checksum; externally changed artifacts fail
rather than silently serving different bytes under an old URI.

Resources are registered for the lifetime of the server process. After restarting,
open the project or build another export to register resources again. Arbitrary
`file://` reads are unsupported. Only registered project snapshots and artifacts
are exposed; the server is not a general filesystem reader.

## Cancellation, boundaries and limits

Request cancellation is passed to shared services, including workspace-lock waits
and active rendering. Cancelling a queued/running durable job with `jobs_cancel`
uses the durable job policy. Cancelling `jobs_run` cancels its active work; other
queued jobs remain queued. A job that already succeeded keeps its result. Poll
the persisted status to resolve races between cancellation and completion.

Paths are checked before service calls, including both current and proposed
document references, sources, fonts, font license companions, masks, manual
selections, build outputs and resource reads. Existing ancestors of new output
paths are resolved. Symlinks and Windows irregular/reparse points within project
trees are conservatively rejected, including links pointing inside an allowed
root. Configure ordinary local directories; project discovery does not follow
symlinks. Queued renderers may read only their frozen dependency files.

This is an application boundary for a trusted local project, not an OS sandbox.
Do not concurrently replace paths/links using external programs during requests;
filesystem preflight cannot prevent hostile time-of-check/time-of-use replacement.
Do not expose this stdio process to remote clients. Configured roots grant access
to project content beneath those directories; use narrow dedicated roots.

An interrupted edit journal must be recovered with a trusted local CLI operation
before MCP access (`recovery_required`). Job owner-death recovery remains automatic.
Concurrent external metadata changes or disappearing temporary files can produce
transient operation errors; poll again if appropriate. Discovery is limited to
1,024 projects, project-tree checks to 100,000 entries, inbound protocol frames to
8 MiB, and individual artifact reads to 32 MiB. Large projects may hit these
limits. There is no daemon scheduling, resource subscription, automatic artifact
cleanup, ComfyUI transport, or distributed execution.
