# Sprint 01 verification

## Red–green evidence

1. Added loader/initialization tests against explicit not-implemented functions. `go test ./internal/project` failed on standalone loading, book loading, validation diagnostics, references and initialization. Implemented strict loading and templates; the same suite passed.
2. Added a CLI init → validate → inspect workflow test. It failed with `unknown command "init"`; added shared application services and command factories, then the full suite passed.
3. Added a numeric-ID regression. The test failed because unquoted `123` was silently coerced to a string. Added strict string-tag validation; the regression passed.

Additional regression assertions cover duplicate references/document IDs and revision invariance. These preserve already implemented behavior rather than introducing further behavior changes.

## Independent review

An independent agent invoked `.agents/skills/paneltree-code-review/SKILL.md` for both the engineering-policy/workflow changes and Sprint 01. It found no actionable correctness issues and independently ran the full Go tests successfully. Its nonblocking coverage suggestions were incorporated.

## Scope and limitations

Local checks passed: `go test ./...`, `go test -race ./...`, `go vet ./...`, golangci-lint v2.14.0 (zero issues), actionlint v1.7.12, and the skill-creator validator. All six CGO-disabled Windows/macOS/Linux amd64/arm64 targets compiled. The native Windows binary validated the embedded two-page project. Changes remain local; Sprint #1 is not closed and hosted workflows have not run.

This sprint provides authoring models, local initialization, validation, and inspection only. Scene/render types are contracts; geometry resolution and rendering remain future work. Asset paths are retained but their files are not validated/rasterized. Root enforcement for remotely initiated operations arrives with workspace/MCP services. Initialization refuses existing targets and retains incomplete new directories for inspection on failure.

Local verification does not imply hosted CI execution or branch protection. Tag workflows produce Actions artifacts and checksums; no tag or public release is created by this work.
