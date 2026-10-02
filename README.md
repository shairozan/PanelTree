# PanelTree

A composition-first sequential-art toolchain written in Go.

Module: `github.com/shairozan/PanelTree`

## Status

MVP (Sprint 09): strict YAML projects, recursive layout, PNG/SVG/basic-Latin lettering, portable editable SVG bundles, incremental builds, revision-aware editing, durable local asset jobs, and a local stdio MCP server. Runs locally without AI services. Sprint 10 adds optional ComfyUI RGB draft generation and explicit selection. A local browser workspace now supports project discovery, character/reference reuse, candidate review, composition and PNG export. PDF and motion exports remain planned.

Start with the [fresh-checkout walkthrough](docs/getting-started.md), [contributing](CONTRIBUTING.md), and [MVP acceptance matrix](docs/acceptance.md).

Original application code is [MIT licensed](LICENSE). Dependencies, fonts and artwork retain their own terms; see [licensing](docs/licensing.md) and [third-party notices](THIRD_PARTY_NOTICES.txt).

## Development

Use Go 1.27.1 or later.

```sh
go run ./cmd/paneltree --help
go run ./cmd/paneltree --version
go run ./cmd/paneltree init my-book
go run ./cmd/paneltree validate my-book/project.yaml
go run ./cmd/paneltree inspect my-book/project.yaml
go run ./cmd/paneltree inspect my-book/project.yaml --width 1080 --height 1920 --fit contain
go run ./cmd/paneltree build my-book/project.yaml --page page-01 --output my-book/page-01.png
go run ./cmd/paneltree build my-book/project.yaml --page page-01 --bundle my-book
go test ./...
go build -o bin/paneltree ./cmd/paneltree
```

On Windows, use `-o bin/paneltree.exe` when building.

Commands are constructed by `internal/cli.Command()`; there is no `init()` registration. Each executable command owns a fresh Viper instance. Its pre-run initializer populates a captured configuration pointer only after validation. `init` requires a new destination directory and never overwrites an existing project. `validate` and `inspect` accept a book, chapter or standalone page.

Read [the schema](docs/schema.md), [architecture and CI](docs/architecture.md), and [engineering requirements](AGENTS.md). The canonical example is embedded from `internal/project/template`; initialization copies it with its fixture asset license.

See [layout and measurement](docs/layout.md) for geometry, transforms and output-fit conventions. The default service measures explicit-font lettering using the same wrapping plan as rendering.

See [PNG rendering](docs/rendering.md) for masks, alpha, limits and output protection. Exports require a new destination file and preserve all source assets and layer definitions.

See [SVG, lettering and editable bundles](docs/editable-bundles.md) for supported SVG coverage, font requirements, portable publication and external-edit limitations.

See [incremental builds](docs/incremental-builds.md) for cache keys, dependency explanations, explicit draft revisions/seeds and recovery. Builds cache by default; `--cache-dir` selects a shared location and `--no-cache` bypasses it. Every export still requires a new destination.

See [revision-aware editing](docs/editing.md) for typed changesets, review/approval,
durable artwork pins, manual overrides, scoped locks, and interruption recovery.

See [durable asset jobs](docs/jobs.md) for renderer discovery, frozen requests,
bounded execution, cancellation, recovery, and explicit candidate selection.

See [ComfyUI generation](docs/comfyui.md) for optional RGB drafts, versioned
profiles, backend/model/seed provenance and an opt-in real-backend smoke test.

See [character packages](docs/characters.md) for shared versioned identities,
selected-state dependencies, reference/license provenance and visual evaluation.

See [approved character references](docs/character-references.md) for multi-view
creation, explicit approval, immutable publication and actual image-conditioned
reuse. The [local evaluation](docs/sprints/22-verification.md) documents the
current SD 1.5 profile's visual limitations.

See [local MCP authoring](docs/mcp.md) for client setup, allowed roots, tools,
resources, previews, cancellation, and protocol error handling.

## Runtime configuration

Optional explicit configuration:

```yaml
log-level: info
```

```sh
go run ./cmd/paneltree --config runtime.yaml --log-level debug
```

Precedence is explicit flags, environment, the selected file, then defaults. `PANELTREE_LOG_LEVEL` sets the log level through the environment. Valid values are `debug`, `info`, `warn`, and `error`; the setting is reserved for service logging as those services are added. No configuration file is required or automatically created. An explicit missing file or unknown configuration field is an error. Help and version do not load configuration.

Runtime configuration is separate from book/page project definitions.

For hosted generation with named profiles, see [Ideogram renderer](docs/ideogram.md) and [example runtime configuration](examples/ideogram/runtime.yaml).

Optional storage: [PostgreSQL projects and shared characters](docs/postgres.md).

Local browser workspace: [setup and user guide](docs/web-workspace.md).
