# Sprint 07 verification

Implementation for GitHub issue #7 is local and uncommitted. No push, issue
closure, release or tag was performed. Hosted checks for this change are unverified.

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
