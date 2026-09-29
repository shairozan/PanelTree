# PanelTree

A composition-first sequential-art toolchain written in Go.

Module: `github.com/shairozan/PanelTree`

## Status

Sprint 01: recursive authoring types, strict YAML loading, project initialization, validation and JSON inspection. Layout, rendering, MCP, and exports are planned and not implemented yet.

## Development

Use Go 1.27.1 or later.

```sh
go run ./cmd/paneltree --help
go run ./cmd/paneltree --version
go run ./cmd/paneltree init my-book
go run ./cmd/paneltree validate my-book/project.yaml
go run ./cmd/paneltree inspect my-book/project.yaml
go test ./...
go build -o bin/paneltree ./cmd/paneltree
```

On Windows, use `-o bin/paneltree.exe` when building.

Commands are constructed by `internal/cli.Command()`; there is no `init()` registration. Each executable command owns a fresh Viper instance. Its pre-run initializer populates a captured configuration pointer only after validation. `init` requires a new destination directory and never overwrites an existing project. `validate` and `inspect` accept a book, chapter or standalone page.

Read [the schema](docs/schema.md), [architecture and CI](docs/architecture.md), and [engineering requirements](AGENTS.md). The canonical example is embedded from `internal/project/template`; initialization copies it with its fixture asset license.

## Runtime configuration

Optional explicit configuration:

```yaml
log-level: info
```

```sh
go run ./cmd/paneltree --config runtime.yaml --log-level debug
```

Precedence is explicit flags, environment, the selected file, then defaults. `PANELTREE_LOG_LEVEL` sets the log level through the environment. Valid values are `debug`, `info`, `warn`, and `error`; the setting is reserved for service logging as those services are added. No configuration file is required or automatically created. An explicit missing file or unknown configuration field is an error. Help and version do not load configuration.

Runtime configuration is separate from future book/page project definitions.
