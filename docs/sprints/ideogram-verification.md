# Ideogram renderer verification

Branch: codex/ideogram-renderer. Base: bcb9e58, the local character-reference
checkpoint on codex/character-reference-workflow. No remote publication or paid
API requests were performed.

## Red/green evidence

Observed behavior failures before their implementations:
- TestIdeogramRuntimeDiscovery: strict config rejected `generation` as unknown.
- TestIdeogramSubmitPollAndResume: adapter scaffold returned `not implemented`.
- TestIdeogramCandidateLifecycle: `renderer ideogram is unavailable`.
- TestRecoverResumableExecution: interrupted remote job recovered as failed,
  expected queued with persisted execution state.
- TestIdeogramReferenceRequest: demanded a ComfyUI profile.
- TestIdeogramCLIProfileFlag: unknown `--generation-profile` flag.
- TestIdeogramEstimateDoesNotGenerate and TestResumeRequiresKnownRemoteExecution:
  declared interfaces returned `not implemented`.
- TestIdeogramCandidateLifecycle extension: provider result metadata missing.
- Review regression: completed candidate acceptance failed after removing the key.
- TestComfyRejectsIdeogramReferenceOptions: new character-image option was ignored.
- TestImageFlagsCannotSilentlyRequestBuiltin: image-only CLI requests silently
  queued builtin work instead of treating the flags as generation intent.
- TestIdeogramDiscoveryReportsImageRoles: missing reference-image capabilities.

Race testing also detected a shared context-variable read/write race in execution
checkpoint setup. A separate immutable handler context fixes it; the existing
job/cancellation race suite supplies the regression coverage.

Each focused test passed after implementation/correction. Additional tests cover
multipart character/style inputs, seed/profile settings, frozen bytes and profile
changes, duplicate requests, provider errors without duplicate submission, pending
cancellation, MCP profile requests and secret-free discovery.

## Original f6be597 checks

Passed on the original 3.0 implementation:
- gofmt and git diff --check.
- go test ./... and go vet ./....
- golangci-lint 2.14.0: zero issues.
- go test -race ./... across all packages.
- Windows amd64 cross-build: bin/paneltree.exe (local ignored artifact).
- Executable discovery using examples/ideogram/runtime.yaml: all three profiles
  reported expected models, operations and reference limits; unavailable without key.
- Independent paneltree-code-review agent review and re-review: no remaining
  actionable findings after corrections.

Docker Go 1.27.1 was used because native Windows Go execution is blocked by host
application-control policy. Hosted CI, native Windows execution and real API
quality/billing remain unverified.

## Deliberate limits

At the original f6be597 baseline only Ideogram 3.0 generate and character were
supported. See the 4.5 follow-up below for the current scope. Automatic dollar caps
remain deferred; raw non-billing estimates are available. Published card selection
is explicit via an image path. Dedicated multi-character binding and transparent
output operations are not implemented.


## Ideogram 4.5 follow-up — 2026-10-01

Requested scope: use 4.5 for manga assets and plan the local anime-checkpoint base
image process, conditional on commercial ownership. Published hosted API terms
were checked before implementation; findings and links are in docs/ideogram.md.
No self-hosted Ideogram weights or anime checkpoint license is assumed approved.

Red evidence before implementation:
- `go test ./internal/adapters ./app -run Ideogram45 -count=1` failed
  TestIdeogram45Transport, the valid generate/edit cases in TestIdeogram45Validation,
  and TestIdeogram45CandidateLifecycle with unsupported renderer/model/operation.
- TestIdeogram45Discovery separately failed because the 4.5 profile was rejected.

Green: focused Ideogram tests passed after model-specific validation, ordered
multipart images, quality/size and endpoint support. Additional edit-candidate
coverage verifies source upload, frozen profile execution and offline selection.
The multipart test uses distinct source/reference bytes to check ordering.

4.5 profiles support text-only generate and edit with one explicit source plus four
supporting images. Existing character/style flags are documented as source/support
roles for 4.5. Size auto/source are supported; custom sizes and masks are deferred.
Existing 3.0 profiles and job wire format remain supported. New profile fields omit
empty JSON values to preserve old profile hashes. Human acceptance remains separate.

The checkpoint workflow is a documented follow-up plan, not an implemented model
installer/orchestrator. Each exact checkpoint/LoRA must pass its own license review.


Current 4.5 validation:
- Full `go test ./...`, `go vet ./...`, golangci-lint 2.14.0 (zero issues),
  formatting and `git diff --check` passed.
- Windows amd64 cross-build and native CLI execution passed. The example runtime
  config reports all three 4.5 profiles with expected edit/source limits.
- Native CLI asset request -> live `jobs estimate` using Patrick returned
  `ideogram_4_5_generate`, quantity 1, exact quote `usd_micros: 220000` ($0.22).
  This was `dry_run=true`; no generation was requested. The local queued test job
  was cancelled immediately afterwards so a later queue drain cannot bill it.
- Independent paneltree-code-review agent found no actionable code defects;
  its requested clarification of baseline versus 4.5 verification was applied.
- Full race verification remains incomplete: initial run crashed in the runtime
  during existing app reference packing; retry hit a runtime internal compiler
  error. A GOMAXPROCS=2 / -p=1 retry built successfully but crashed in PNG/flate
  processing in app/MCP. No race report was emitted. These toolchain/runtime faults
  have not been attributed to a code change; tests were not suppressed or weakened.
  Hosted CI has not run and remains a required gate before merge.

- Focused affected-path race check passed: `go test -race -count=1
  ./internal/adapters ./app -run Ideogram` with GOMAXPROCS=2. This does not replace
  the incomplete full race gate.
