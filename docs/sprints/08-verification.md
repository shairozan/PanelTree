# Sprint 08 verification

Issue #8 implementation reached hosted CI, which exposed the Windows polling
race documented below. That correction is local and uncommitted; its hosted
checks remain unverified. No issue closure, release or tag was performed.

## Red–green evidence

- MCP client workflow and policy tests first failed with `MCP server not
  implemented`. They now pass over the official SDK's in-memory transport.
- The real stdio client initially failed initialization with `unknown flag:
  --root`; the CLI root test failed with `unknown command "mcp"`. The same client
  now initializes a book, edits a layer, runs an asset job, builds a page and reads
  its preview through the `mcp serve` command.
- Runtime configuration initially rejected `mcp-roots` as an unknown key. The
  typed setting now passes through the invocation-local initializer.
- Independent review's nested-owner regression failed with `outside_root` despite
  a valid self-owned project. Removing the unrelated ancestor-owner check fixes it.
- An implicit font-companion boundary test failed because the `.LICENSE` path was
  not checked. The fix checks companions and rejects Windows irregular/reparse
  points as well as symlinks. A Unix-capable integration test additionally checks
  that an out-of-root font-license symlink cannot leak into an exported bundle.
- A persisted job payload with an absolute source absent from frozen inputs
  incorrectly succeeded. The shared runner now rejects that request; every source
  and font must be a safe relative path present in the frozen file map.

Schema inference initially rejected recursive layout models; explicit referenced
JSON Schemas now preserve their typed shape. Nullable collections and custom
serialized scene values are represented without flattening authoring documents.
Windows fixture-access and symlink-privilege failures are environment limitations,
not red evidence. The Windows junction test exercised the boundary without
requiring file-symlink privilege.

## Coverage and checks

The real SDK client uses a subprocess stdio transport against the Cobra command
factory. Its PNG bytes and revision match a separate CLI build. Client tests also
cover tool/resource discovery, typed application errors, stale revisions, lock
bypass rejection, unavailable generators, out-of-root paths, indirect references,
persisted manual overrides, changed artifact checksums, and bundle SVG retrieval.

Cancellation tests exercise a queued durable job and a protocol request blocked
on a real workspace lock. A subsequent gated operation verifies that cancelled
requests release the MCP service gate. Active queue execution with repeated status
and cancellation calls passed ten repetitions locally. That initial passing stress
run did not catch the traversal race later reproduced in CI; see the correction
below. Shared job tests cover running cancellation and owner death.

- `gofmt` applied to changed Go sources.
- `go test ./... -count=1`: passed.
- `go vet ./...`: passed.
- CI-pinned `golangci-lint v2.14.0 run`: passed, 0 issues.
- MCP tests cross-compiled for Linux and macOS; not executed on those hosts locally.
- `go test -race ./...`: blocked because local CGO is disabled; hosted race checks
  remain required.
- `git diff --check`: passed.

Independent review used `.agents/skills/paneltree-code-review/SKILL.md`, inspected
actual changes and requirements, and re-reviewed the boundary corrections. The
reviewer independently ran the MCP tests with required filesystem permissions;
they passed. Final documentation review is complete; no actionable findings remain.

See [MCP usage](../mcp.md) for root policy, protocol envelopes, startup, previews,
recovery and remaining limits. The SDK's stdio protocol handles negotiation and
cancellation; no ComfyUI or external model service is required for these tests.

## Windows CI polling correction

Hosted `TestRunningQueuePolling` failed when a worker removed a `.render-*`
directory after tree enumeration but before the scanner opened it. The earlier
review had flagged this scheduling possibility; local stress testing had not
reproduced it.

The deterministic regression removes an actual empty render directory between
the walk callback and descent. Before the fix it failed with the same
`open ... .render-completed: The system cannot find the file specified` error.
Tree scanning now ignores only not-found errors for vanished entries and continues
to inspect surviving siblings. Permission failures and other storage failures
are still returned, and symlink/reparse checks remain enforced. Required project
inputs still fail through their readers if missing.

After the fix, the deterministic tests, active queue polling, and Windows
implicit-dependency boundary tests passed 20 repetitions. Full
`go test ./... -count=1`, `go vet ./...`, and pinned lint (0 issues) passed.
Independent correction and documentation review found no actionable issues;
`git diff --check` passed. Hosted CI needs to rerun against the
corrected commit; the local CGO/race limitation is unchanged.
