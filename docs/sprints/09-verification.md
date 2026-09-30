# Sprint 09 verification

## Scope and test discipline

This sprint adds acceptance/fuzz coverage, portability gates, licensing and contributor documentation around already-implemented MVP behavior. The new executable acceptance and fuzz seed tests passed against the existing implementation; no artificial red phase was introduced. No production Go behavior changed. Configuration changes were checked using the actual build/fuzz commands, actionlint and Git attribute inspection.

The owner explicitly selected MIT on 2026-09-30 after clarifying that MIT and BSD-3-Clause are separate licenses. LICENSE records MIT for original application code; third-party dependency, Go runtime, font and artwork notices remain intact. Windows/Linux/macOS amd64/arm64 dependency inventories contained no unreviewed additional module. The independent reviewer compared all 25 module license/NOTICE sections against their pinned downloaded originals.

## Acceptance and visuals

`acceptance/TestMVPExecutableWorkflow` compiles a fresh executable and exercises CLI init/validation of the licensed two-page book; PNG and editable bundles for both pages; equivalent warm/portable pixels; actual stdio MCP initialization, asset request/run/status/selection; stale-revision rejection; approval and all-lock enforcement across interfaces; PNG/SVG artifact resources; durable cancellation; and byte-preserved authored inputs. Existing cross-package tests cover true source-hidden bundle relocation, selective invalidation, manual/durable ownership, scoped locks, interrupted transactions, idempotency and process-death recovery. See [the matrix](../acceptance.md).

Built both pages at 800×1200 and inspected their PNGs and portable SVGs in the browser. Page 1 retained the five-panel composition, caption, lantern, FX, rotation, translucent character and oval alpha mask. Page 2 retained the isolated character/cutout. Native SVG text and relative assets displayed correctly; browser edge smoothing differs from nearest-neighbor PNG sampling, as documented.

The first local SVG preview hit Python's legacy Windows path-length handling and showed missing assets. Using an extended-length server root resolved all asset requests; no exporter change was needed. This external-tool limit is recorded in `docs/editable-bundles.md`. The loopback review server and temporary browser tab were stopped after inspection.

Local ignored visual artifacts: `bin/sprint-09-page1.json`, `bin/sprint-09-page2.json`, the corresponding `bin/sprint-09-demo/` bundles, and `bin/sprint-09-svg-page1.png` / `bin/sprint-09-svg-page2.png` screenshots.

## Checks

- `go test -count=1 ./...`: all packages passed, including executable acceptance and recovery tests.
- `go test -race -count=1 ./...`: all packages passed locally on Windows.
- `go vet ./...`: passed.
- CI-pinned golangci-lint v2.14.0: zero issues after normalizing existing CRLF Go working files. `.gitattributes` now enforces LF for Go on every platform; existing Go files have no semantic diff.
- Bounded fuzzing, 10 seconds per target, two workers: strict YAML passed (370 executions); layout passed (199,034 executions). These short runs add coverage, not proof against all malformed inputs.
- CGO-disabled executable compilation passed for Windows, Linux and macOS, each on amd64 and arm64. This is cross-compilation, not native execution on every OS.
- actionlint, Go-source formatting and `git diff --check`: passed. CI adds native executable builds and fuzzing; reusable release gates still run before binary/license/notice packaging.

## Review and remaining limits

Independent review used `.agents/skills/paneltree-code-review/SKILL.md`; acceptance/project/layout tests were independently rerun and notices checked. No actionable findings remained. Runtime limitations remain explicit: basic-Latin lettering, bounded SVG shapes, no external-editor changeset import, local trusted-root MCP rather than an OS sandbox, full-output cache surfaces, and no automatic cache/job cleanup. Generation, web editing, PDF, motion and release publishing are outside this sprint, not deferred MVP acceptance work.

Changes are local for review and push. Native hosted CI for this exact change set remains pending; this record does not claim a hosted pass. Sprint 09 has not been closed and no release/tag was published.
