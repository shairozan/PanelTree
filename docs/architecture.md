# Sprint 01 contracts

- `model`: versioned authoring data and stable ID/revision/state/lock types, with no renderer-vendor fields.
- `scene`: geometry, measurement/layout interfaces and resolved-node skeletons. No layout algorithm is implemented yet.
- `render`: leaf descriptor, declared dependencies, typed request and artifact interfaces. No renderer is implemented yet.
- `internal/project`: strict loading, semantic validation, ordered reference resolution and creation from an embedded canonical fixture.
- `app`: typed Init/Validate/Inspect operations callable by CLI or future MCP/web adapters.
- `internal/cli`: command factories with independent Viper/pre-run configuration pointers and JSON results. Commands do not duplicate parsing logic.

The loaded document tree remains separate from the future resolved scene and build graph. No geometry, generated pixels, cache hits, edits or approval enforcement are implied by the contract skeletons. Evolve the public pre-1.0 types through runnable fixtures rather than treating them as a frozen plugin ABI.

## Development gates

Follow `AGENTS.md`: mandatory red–green TDD, independent review using `.agents/skills/paneltree-code-review/SKILL.md`, formatting, tests, vet and golangci-lint. Install the CI-pinned linter with:

```sh
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0
golangci-lint run
go test ./...
go vet ./...
```

CI runs on every push/PR, with native tests on Windows, macOS and Linux and race tests on Linux. The release workflow runs the full CI suite before cross-compiling six CGO-disabled binaries (amd64/arm64 for Windows/macOS/Linux). A pushed version tag such as `v0.1.0` produces downloadable Actions artifacts and per-binary SHA-256 checksums. It does not automatically publish a GitHub Release or push a tag. Hosted execution and branch protection are separate from committing workflow files.
