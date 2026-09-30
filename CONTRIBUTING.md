# Contributing

Use the [fresh-checkout walkthrough](docs/getting-started.md). Work from a focused issue/branch, keep changes within acceptance criteria, and preserve unrelated work and authored assets. Original contributions are under the repository's MIT license; contribute only material you have permission to license. Dependencies, fonts and artwork retain their own licenses.

Read [AGENTS.md](AGENTS.md). For behavior changes, write a focused failing test, record its observable failure, implement the fix and rerun it. A tooling/compiler failure is not red evidence. Acceptance tests for already-correct behavior may pass immediately; do not manufacture failures. Configuration/documentation changes need appropriate executable validation.

Before completion, request independent agent review using [.agents/skills/paneltree-code-review/SKILL.md](.agents/skills/paneltree-code-review/SKILL.md). Supply the ticket, actual diff and test evidence. Fix actionable findings and request re-review of substantive changes. Disclose blocked gates rather than substituting self-review.

```sh
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0
golangci-lint run
gofmt -l .
go test ./internal/project -run '^$' -fuzz '^FuzzStrictDocument$' -fuzztime=10s -parallel=2
go test ./internal/layout -run '^$' -fuzz '^FuzzLayoutGeometry$' -fuzztime=10s -parallel=2
```

Format changed Go files with `gofmt -w`; the listing must be empty. Race tests require a supported CGO/C toolchain; report local limitations and verify Linux CI when unavailable. Fuzzing bounds time, worker count and inputs. Preserve discovered failing corpora and follow red–green repair. `go test ./acceptance -count=1 -v` runs the compiled CLI/MCP workflow without a model.

Keep policy in shared `app` services. CLI factories use invocation-local Viper and typed configuration pointers initialized in `PreRunE`; no `init()` registration. MCP enforces roots and shared policy. Adapters consume final scene context and return independent assets; they cannot silently approve, unlock or replace manual artwork. See [architecture](docs/architecture.md).

For visual changes, inspect PNG and editable SVG and record dimensions, artifacts and limitations. Pixel hashes alone do not establish visual correctness. For dependency updates, inspect upstream license/NOTICE changes, refresh `THIRD_PARTY_NOTICES.txt`, retain asset/font attribution and check target-specific dependencies on all supported platforms.

PRs should explain the problem and resulting behavior, include red–green/acceptance evidence and review results, and distinguish local checks from hosted CI. Version tags build binary artifacts only after all gates pass. Do not push tags or publish releases without explicit authorization.
