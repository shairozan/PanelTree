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

## Checks

Passed on the final implementation:
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

Only Ideogram 3.0 generate and character operations are supported. Automatic dollar
caps are deferred until the PriceQuote schema is verified; raw non-billing estimates
are available. Published card selection is explicit via an image path. Multi-character
conditioning, transparent outputs and editing operations are not implemented.
