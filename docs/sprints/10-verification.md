# Sprint 10 verification

Scope: GitHub issue #10, ComfyUI leaf generation and explicit draft selection.
Implementation is on local branch `codex/sprint-10-comfyui`.

## Red–green evidence

- `go test ./internal/cli -run TestComfyRuntimeDiscovery -count=1` failed
  because runtime decoding rejected `comfyui-url` and `comfyui-profile`.
  After adding invocation-local configuration and profile loading it passed;
  a second invocation proves the backend setting does not leak.
- `go test ./internal/adapters -run TestComfy -count=1` initially failed
  against interface stubs. After implementing profile freezing, the recipe
  tests passed while HTTP tests still failed with `not implemented`, zero
  submissions and missing timeout/cancellation behavior. Implementing the
  transport made the suite pass: success, read retry, timeout, cancellation,
  submission/execution errors, ambiguous submission, invalid/wrong-sized or
  multiple images, cleanup failure and opaque RGB normalization.
- `go test ./app -run TestGenerated -count=1` failed with
  `renderer_unavailable` before the ComfyUI job path was implemented. It now
  verifies duplicate requests, changed-seed conflicts, explicit selection,
  preserved authored YAML, composition, model/profile invalidation, and protected
  selections. The first approval fixture lacked a review state/artifact; those
  fixture setup errors were not counted as red evidence. Existing approval
  policy itself was retained unchanged.
- `TestGeneratedCandidateWorkflow` subsequently failed with `job status omits
  generation provenance`; job responses now expose the frozen backend/profile/
  model/seed/dimension provenance and the test passes.
- `TestCancellationPreservesBackendCleanupDiagnostic` failed because the durable
  store discarded a renderer's cancellation detail. It now preserves that detail
  without publishing a result, and the test passes.
- `TestComfyHTTP/submit_error` failed because HTTP 400 lost `invalid workflow`.
  Bounded backend error text is now retained in diagnostics.

The initial sandboxed Go invocation failed to access the toolchain/build cache;
that was an environment failure, not a red test. Subsequent Go checks use the
accessible installed toolchain.

## Verification

Affected service, CLI, MCP, config and adapter suites passed. The CLI test also
starts a real stdio MCP subprocess with runtime configuration and exercises
generation, duplicate requests, capability discovery, provenance and selection.
Terminal failed/cancelled jobs do not resubmit under the same key. Existing
static-asset behavior, approvals and locks retain their original policy tests.

- `go test ./... -count=1`: all packages passed, including executable acceptance.
- `go test -race ./... -count=1`: all packages passed locally on Windows.
- `go vet ./...`: passed.
- `golangci-lint run`, CI-pinned v2.14.0 built with Go 1.27.1: zero issues.
  Its initial run found three numeric HTTP status literals in test fixtures;
  those were replaced with named `net/http` constants, with no suppression.
- `gofmt -l app internal render` and `git diff --check`: clean.
- `TestComfyRealBackendSmoke`: explicitly skipped with no opt-in environment.

Independent review used `.agents/skills/paneltree-code-review/SKILL.md`, inspected
the complete tracked/untracked diff and acceptance/TDD evidence, and found no
actionable issues. The reviewer independently passed:
`go test ./internal/adapters ./app ./internal/cli ./internal/jobs -run 'TestComfy|TestGenerated|TestCancellationPreserves' -count=1`.
The reviewer also confirmed the atomic scoped cancellation endpoint against
upstream ComfyUI. No substantive corrections were requested.

## Limits and external checks

No real ComfyUI endpoint, model or GPU was supplied, so the opt-in real-backend
smoke test has not run. `docs/comfyui.md` and `TestComfyRealBackendSmoke` give a
reproducible procedure retaining the generated page and provenance for review.
The example profile must be filled with installed model names/hashes and runtime
identity. Model/backend identities are operator assertions, not remote checksum
attestation. No artistic quality or consistency guarantee is made.

Only RGB drafts are supported. Unexpected alpha is flattened against white;
isolated-RGBA requests are rejected. POST submission is never blindly retried;
ambiguous acceptance or process death can leave backend work running. Cancellation
requires the modern atomic prompt-scoped endpoint; older servers produce a
cleanup diagnostic instead of risking interruption of unrelated work. Read
retries and time/size/dimension limits are bounded. These limits are documented.

All changes are local. Hosted CI has not run for this change set. No issue has
been closed, no PR or release published, and no tag pushed.
