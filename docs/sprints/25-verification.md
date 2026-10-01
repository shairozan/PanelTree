# Issue 25 verification

Scope: optional PostgreSQL authoring storage and shared immutable character library.
Branch: `codex/postgres-storage`, based on main `3ac370d`.
No UI or paid renderer calls. No issue closure, remote push or release in this work.

## Red/green evidence

Focused tests were run before each implementation. Initial storage/library stubs
returned `not implemented` at runtime; compilation failures were not counted as red.

| Observable behavior | Observed red | Green coverage |
| --- | --- | --- |
| File and PostgreSQL project contract | `not implemented` (both; PG with real server) | import/open/collision/export and identical revision |
| Shared app operations on pg handles | attempted filesystem lstat of `pg:...` | inspection, edits, stale revision refusal, build |
| Durable jobs | `not implemented`; later deleted scratch metadata lock | checkpoint resume, frozen-input conflict, request/run/select |
| Immutable library and project binding | `not implemented` | two projects, pinned version, reject deletion/overwrite, locked upgrade refusal |
| CLI and MCP | unknown storage command; pg handle outside_root | configured CLI migration/init/inspect and MCP init/open/list |
| Portable queued jobs | imported job not_found | retained cancelled hold, export/reopen |
| Referenced files | custom extension lost; escaping path accepted | dependencies preserved, outside project rejected |
| Clean restore | library provenance lost in fresh schema | restore bindings, original bytes and deletion protection with new blob root |
| Backup integrity | existing library accepted tampered bytes | same-version imports verify hashes too |
| Job input isolation | external symlink imported | bounded safe reads reject linked input |
| Stable inspection | scratch paths returned for documents/pages/scenes | project-relative paths |
| Manual override portability | later operation could not open removed scratch path | selected bytes and locks survive source move and portable export |
| Alternate owner filename | exported project.yaml missing | normalize root document name, preserve relative dependencies |
| Worker connection loss | stale checkpoint accepted and stale artifact succeeded | terminate matching task-owned executor session; competing runner blocked, superseded writes rejected, unknown request not resubmitted |
| Unsupported schema | future schema library/list/jobs accepted | all service connections reject unsupported versions |
| Character dependencies | credentials in private.env copied; custom package dependency lost | skip unrelated env file, retain custom-extension package references |
| Portable override reselection | old mapped artwork silently retained | explicit new selection gets immutable content locator |
| Independent overrides | unlocked selection changed locked sibling | per-selection immutable locator preserves sibling and original mapping |

Additional preservation/failure tests exercise eight accepted reference views, four
cards, license/attribution and candidate lineage, reuse by a second project,
approval pins and all locks, file/DB/file/DB round trips, interrupted workspace
rollback and missing-blob recovery. These verify existing intended behavior without
manufacturing production changes to force red.

## Checks

Tests use a disposable PostgreSQL 18 container on a private Docker network, no host
port. Linux Go 1.27.1 matches go.mod. No production database or renderer was contacted.

- `go test -count=1 ./...` with `PANELTREE_TEST_POSTGRES`: passed.
- `go vet ./...`: passed.
- `golangci-lint run` v2.14.0 (CI-pinned): passed, zero issues.
- `gofmt` and `git diff --check`: passed.
- Windows amd64 cross-build with CGO disabled: passed.
- actionlint v1.7.7: passed (optional shellcheck/pyflakes disabled).
- Full default-parallel race attempt: failed with Go runtime SIGSEGV/faults in app
  and CLI, plus malformed MCP JSON; no clean full race result is claimed for that run.
- Final full serial race rerun: `GOMAXPROCS=1 go test -race -p=1 -count=1 ./...` with PostgreSQL enabled passed every package. This does not erase the earlier default-parallel failure.
- Hosted CI, Windows/macOS execution and release gates: not run locally. The existing
  cross-platform jobs remain; a real PostgreSQL full-suite/affected-race CI job was
  added and is also required by the reusable release checks.

## Independent review

The independent paneltree-code-review agent inspected the actual tracked/untracked
diff and requirements. Fixed its findings on manual override portability and alternate
owner names, then its follow-up findings on manual alias reselection and locked sibling
mutation. It re-reviewed the immutable-selection correction and reported no remaining
actionable code findings. The reviewer also checked schema gating, rollback/missing-blob coverage, verification/setup documentation and new dependency license notices; no additional findings remained. Final passing serial-race result was recorded afterward.

## Operational limits

Trusted single-operator deployment. All processes must use the same durable blob root
and direct/session-pooled PostgreSQL connections. Portable import/export is explicit;
there is no automatic file/database synchronization. Database records and blobs must
be backed up together with writers stopped. Old library bindings/orphan blobs are
retained conservatively; garbage collection, cloud blob storage and UI are out of scope.
Current storage materializes operation-local files to keep renderer contracts unchanged.


## Follow-up: CI race failures

The hosted race log supplied by the user showed the Ideogram success-path test
exceeding its one-second request deadline, and the MCP cancellation test exceeding
a two-second watchdog that included server initialization.

- Pre-encode the unchanged 1024x1024 PNG fixture before the Ideogram request and use
  a ten-second success-path budget. Preserve submit/poll/resume, exactly-one-paid-
  submission and ambiguous-submission refusal assertions. Production deadlines and
  the separate pending-request timeout test are unchanged.
- Synchronize MCP cancellation with the first input read, then assert input Close
  was called. This stronger test failed before implementation with `input not closed`.
  The server previously hid the pipe/stdin Close method behind io.NopCloser. Preserve
  io.ReadCloser inputs so transport cancellation can close them. Watchdogs allow ten
  seconds for CI scheduling; no sleep determines when cancellation happens.
- Local follow-up validation on Go 1.27.1 with GOMAXPROCS=2: focused race tests
  passed ten repetitions; `go test ./...`, `go vet ./...`, CI-pinned golangci-lint
  v2.14.0 (zero issues), and `go test -race -count=1 ./...` all passed. No package
  serialization flag or CI configuration change was used. PostgreSQL integration
  tests were not enabled for this transport/test-only follow-up.
- Independent code and documentation review passed with no actionable findings.
  Hosted rerun is not yet verified.
