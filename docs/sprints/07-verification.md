# Sprint 07 verification

The initial implementation reached hosted CI, where Windows and macOS job tests
failed on filesystem path aliases. The portability correction below is local and
uncommitted; hosted checks for that correction remain unverified. No issue closure,
release or tag was performed as part of the correction.

## Red–green evidence

- Durable store tests first failed with `durable job submission not implemented`;
  submission, idempotency, execution, cancellation and recovery now pass.
- Application request tests first failed with `asset jobs not implemented`, and
  capability discovery failed with `renderer discovery empty`. PNG, SVG and text
  requests now produce selectable draft candidates through shared services.
- Retrying an identical request after an edit first failed with
  `revision_conflict: asset submission requires current project revision`.
  Looking up the original idempotent request before checking the current revision
  fixes that regression while rejecting conflicting reuse.
- CLI integration first failed with `unknown command "renderers" for "paneltree"`;
  renderer discovery and the request/run/status/select workflow now pass.

Windows sandbox fixture-access failures and temporary-executable application
control failures were environment failures, not red evidence. Focused tests were
then executed with the required filesystem permissions or a workspace test binary.

## Acceptance coverage

Tests cover concurrent duplicate submission, conflicting idempotency keys,
bounded execution, exclusive runner ownership, queued/running cancellation,
renderer failure and partial-output rejection. Recovery is tested with abandoned
state and by killing a real worker subprocess, then observing `interrupted`.

Application tests cover all three available source kinds, unavailable renderer
diagnostics, original revision retention after edits, execution from frozen bytes,
stale revision rejection, changed source bytes at the same revision, lock policy,
and explicit selection. CLI tests exercise the end-to-end workflow and invalid
worker counts. Existing editorial tests continue to cover approval/manual and
scoped lock protections used by candidate selection.

## Checks

- `gofmt` applied to changed Go sources.
- `go test ./... -count=1`: passed.
- `go vet ./...`: passed.
- CI-pinned `golangci-lint v2.14.0 run`: passed, 0 issues.
- `git diff --check`: passed.
- Job-store tests cross-compiled for Linux and macOS; not executed there locally.
- `go test -race ./...`: blocked because local CGO is disabled. Hosted Linux
  race verification remains required.

Independent review used `.agents/skills/paneltree-code-review/SKILL.md` and found
no actionable code issue. Final documentation review is complete; its example
panel-ID correction was applied. No actionable findings remain. The reviewer's
own test attempt encountered sandbox fixture-access failures; runtime evidence
above comes from the implementing agent's successful checks.

## Limits

See [durable asset jobs](../jobs.md) for foreground execution, explicit retries,
renderer-version compatibility, conservative dependency resolution and storage
ownership. Recovery covers process interruption, not power loss or hostile store
modification. There is no daemon, remote scheduler, ComfyUI transport or automatic
job garbage collection. Candidate completion never changes selected artwork.

## CI path-alias correction

The store rejected any root whose resolved spelling differed from its original
path. Valid Windows short names and Unix symlinked parent directories therefore
failed with `job store must not traverse symlinks`. `Open` now retains the resolved
root for subsequent storage and lock operations. Application workspace checks
still reject symlinks within the project-owned job path.

`TestOpenThroughParentAlias` reproduced that exact behavioral failure on Windows
using a short-name alias before the production fix, then passed after it. The
Unix fixture uses a symlinked parent. The test also verifies canonical root
identity and durable job reuse through both names. Windows filesystems without
short-name aliases skip that fixture. Earlier junction-fixture resolution errors
were environment failures and are not counted as red evidence.

After the correction, full `go test ./... -count=1`, `go vet ./...`, pinned lint
(0 issues), and `git diff --check` passed. Job tests cross-compiled for Linux and
macOS; those binaries were not executed locally. Independent skill-based review
found no actionable issues. Hosted Windows/macOS tests and Linux race checks
must still pass on the corrected commit.
