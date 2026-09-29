# PanelTree engineering requirements

These requirements apply to all work in this repository.

## Sprint workflow

Take the lowest-numbered open GitHub issue titled `Sprint ...` when asked for the next item. Read its acceptance criteria, preserve unrelated work, and keep implementation within its scope. Do not close an issue until its implementation and verification are complete. Disclose local-only changes and unverified hosted checks.

## Mandatory red–green TDD

For each behavior change or bug fix:

1. Write a focused test expressing the expected observable behavior.
2. Run it and record the expected failure (red) before production implementation. A tooling/environment failure does not count as red.
3. Implement the smallest coherent change that passes (green).
4. Refactor while keeping tests green, then run affected and full checks.

Include red/green evidence in the work summary or PR. Do not implement first and backfill tests. Documentation-only changes need review; configuration/workflow changes need applicable validation and executable checks, not artificial text-matching tests. Existing behavior being preserved need not be rewritten merely to manufacture a red result.

## Mandatory independent agent review

Before marking work complete, invoke the repository's [paneltree-code-review skill](.agents/skills/paneltree-code-review/SKILL.md) through an independent review agent. This instruction authorizes that review delegation; it does not authorize implementation delegation or external publication.

The reviewer must inspect the actual diff, requirements and test evidence. Address actionable findings, rerun affected checks, and request re-review of substantive corrections. Report review completion and residual risks. If the agent capability is unavailable, explicitly report review as blocked; self-review does not satisfy this gate.

## Required checks

- Format Go changes with `gofmt`.
- Run `go test ./...`, `go vet ./...`, and `golangci-lint run` using the version pinned in CI.
- Run race tests where the toolchain/platform supports them; document any local limitation and rely on the CI race job for the required hosted gate.
- Do not suppress lint findings or weaken tests to obtain a passing build without a justified, documented reason.

## CI and releases

GitHub Actions must run tests on every pull request and push, including tags and documentation changes; no path-based test skips. Lint and test failures block release packaging. Pushing a semantic-version tag such as `v0.1.0` triggers Windows, macOS and Linux binaries with checksums. Do not push tags or publish releases unless requested. Workflow files alone do not prove hosted checks passed.

## Architectural conventions

Use `github.com/shairozan/PanelTree` as the Go module. Compose Cobra commands with factories, never `init()` registration. Use invocation-local Viper and pre-run initialization into typed configuration pointers. Keep runtime configuration separate from strict project YAML. CLI, MCP and future web interfaces use shared services. Generation stays behind leaf renderer contracts. Preserve original assets, manual overrides, pins and scoped locks.
