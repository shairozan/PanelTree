# MVP acceptance matrix

Existing-assets authoring is complete without generation, web editing, PDF or motion output. Those later features are not prerequisites for these acceptance cases.

| Capability | Executable evidence |
| --- | --- |
| Fresh binary and two-page book | `acceptance/TestMVPExecutableWorkflow`: compile, CLI init/validate, both bundles, relocated YAML and image checks |
| Real MCP/CLI workflow | Same acceptance test and `internal/cli/TestMCPStdioAndCLIEquivalence`: stdio client, edits/jobs, PNG/SVG resources |
| Recursive layout, rendering, masks, lettering | `internal/layout`, `internal/compose`, `internal/adapters`; bounded parser/layout fuzzing |
| Selective invalidation | `app/TestIncrementalBuild`, `TestCacheRelocationAndMask`, `internal/build/TestDependencyGraph` |
| Approval/manual ownership and locks | `app/TestEditorialApproval`, `TestManualOverridePreserved`, `TestScopedLocks`, `TestPinnedBuildRetainsApprovedArtwork`, `TestMissingPinNeverRenders`; cross-interface lock check in acceptance |
| Revision conflicts and atomic edits | `internal/workspace/TestRevisionConflict`, `TestValidateEntireChangeset`, `TestCancelledCommitLeavesOldRevision`; stale MCP edit in acceptance |
| Interrupted edits and owner recovery | `internal/workspace/TestRecoveryFinishesInterruptedPublication`; `internal/jobs/TestProcessDeathRecovery`, `TestRecoveryAbandonedOwner` |
| Idempotency, cancellation, result ownership | `internal/jobs/TestDurableIdempotency`, `TestBoundedExecutionAndOwnership`, `TestCancellationAndFailureNeverPublishResults`; persisted CLI/MCP cancellation in acceptance |
| Cache corruption and interrupted publication | `internal/cache` and `internal/build` tests |
| Root/resource boundaries | `internal/mcp` policy/path/tree tests on native Windows and Unix |
| Editable portability/protected destinations | `app/bundle_test.go`, `internal/export`; visual inspection recorded per sprint |

Run `go test -count=1 ./...` for this matrix, including process-crash recovery and the freshly compiled executable. Run race, vet, lint, formatting and bounded fuzzing as documented in [CONTRIBUTING.md](../CONTRIBUTING.md).

Image checks require correct dimensions, multiple colors, opaque page-1 background, transparent page-2 cutout, and identical cold/warm/relocated pixels. Visual review separately checks panel order, lettering, masks, transforms/opacity and editable SVG display. Browser antialiasing/text can differ. Basic Latin and bounded SVG shapes only; no arbitrary SVG import or external-editor changeset round-trip. See [export limits](editable-bundles.md).

CI runs all packages and builds natively on Windows/Linux/macOS. Linux also runs race, lint and two ten-second fuzz targets with two workers. Release tags reuse the whole gate; no path-based test skips. Local passes do not establish hosted results; verify the exact pushed commit/run before closing a sprint.
