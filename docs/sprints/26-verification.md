# Issue 26 verification

Implemented on `codex/visual-workspace`, based on main `bc47d80`. Changes are local;
no pull request, hosted check or issue closure is claimed here.

## Scope and acceptance mapping

- Embedded browser workspace and `paneltree serve` call shared app services for
  filesystem and PostgreSQL projects. Host configuration owns credentials,
  renderer profiles, roots and database connection settings.
- Ordered book/chapter/page navigation, preview without generation, panel/layer
  addition and ordering, lettering, frames/transforms, review, approval, scoped
  locks, revision-safe undo/redo and PNG export cover the initial composition loop.
- Original PNG import retains provenance and never implicitly selects an image.
  Candidate jobs separate preparation/estimate, explicit execution and selection.
  Source/result comparison, durable rejection, cancellation and safe resumption
  preserve originals and known provider execution identities.
- Character authoring, all eight reference views, explicit approval and four-card
  publication feed immutable shared packages. PostgreSQL library versions can be
  copied into file projects; portable bindings survive import back into PostgreSQL.
  Publishing a newer version never upgrades a story automatically.
- Renderer capabilities, source-first references, quality/size, provider usage
  links and unknown-license notices are visible. Anime checkpoint images remain
  an upstream local authoring step; they are imported as originals, not loaded
  into the hosted Ideogram renderer.
- Loopback/Host/Origin/CSRF checks, bounded PNG media, canonical authorized roots,
  opaque artifacts and server-only provider requests constrain local access.

Issue #13 overlap: discovery, ordered navigation and build/export are implemented.
Issue #14 overlap: basic composition, lettering, review and locks are implemented;
advanced mask editing remains outside #26. Neither #13 nor #14 is closed here.
See the [workspace guide](../web-workspace.md) and
[architecture decision](../adr/003-local-web-workspace.md).

## Red–green evidence

Focused tests were run before each production behavior change. These were actual
behavior failures; missing toolchains/browser binaries and test-fixture mistakes
were not counted as red results.

| Behavior | Observed red | Green verification |
| --- | --- | --- |
| HTTP project journey | Stub returned 501 | Create/open/preview, security and roots tests |
| PNG import | Import stub returned not implemented | Original bytes, provenance, no implicit selection; both backends |
| Serve command | Unknown `--root` | CLI lifecycle and loopback tests |
| One confirmed job | RunOne executed two queued jobs | Other prepared job remains queued |
| Editing/history | Edit endpoint 404 | Revision, undo/redo, locks and external conflicts |
| Path boundary | Traversal reached app and returned outside-root lstat | Rejected at boundary with 403 |
| Upload/reference API | JSON-only upload and unknown reference action | PNG upload, revision and approval workflows |
| PostgreSQL provenance | Imported metadata missing | Durable original metadata |
| Shared character reuse | Filesystem reuse unsupported | Immutable copy, collisions, locks and portable binding |
| Character authoring | Not-implemented stub | Validated immutable package creation |
| Library HTTP | Unknown request schema | Create/publish/list/detail/use |
| Frozen job source/settings | Source stub; capability fields missing | Frozen pixels, quality and size |
| Slow preview | Session blocked for one second | Session remains responsive |
| Landscape preview | Incorrect aspect | Canvas-derived aspect and bounded export dimensions |
| Shutdown/worker errors | Closed server returned 200; busy worker hidden | 503 shutdown, visible start_failed diagnostic |
| Portable bindings | PostgreSQL import lost copied library binding | Binding and deletion protection retained |
| Reference workshop | Missing controls | Eight slots, explicit accept and published cards |
| Candidate approval | Approval replaced selected candidate with original | Selected candidate pin retained |
| Missing image | No visible affected-artifact error | Named missing-media error |
| Thumbnail | Original 512x768 pixels returned | At most 256px thumbnail; original untouched |
| Reordering/navigation | Missing reorder controls/book title | Ordered panels, book/chapter/page and bound character details |
| Rejection | Reject endpoint 404 | Durable rejected flag, image retained, selection refused |
| Shared reference cards | Published cards absent from detail | Directional cards exposed and viewable |
| Accessibility | WCAG contrast violations | Axe WCAG2A/AA clean at desktop and 720px |
| Narrow layout | Add panel extended outside outline | Wrapping heading; bounds assertion passes |
| Provider disclosure | Reference workshop usage link absent | Selected profile attribution/policy shown |
| Manual approval | No-artifact approval read a directory | Current manual selection resolved by shared service |
| Missing RunJob ID | Both queued jobs executed | Error and both jobs remain queued |
| Existing fontless project | Add panel returned 409: text requires font | Blank-image layers and licensed font defaults; three add controls pass |
| Overlapping actions | Browser undo/redo reported nothing to redo | Actions queue; held background poll leaves editing controls enabled |

## Checks

- Linux Docker Go 1.27.1: `go test -count=1 ./...` with PostgreSQL 18 enabled:
  passed. `go vet ./...`: passed. `golangci-lint` v2.14.0: zero issues.
- Go sources formatted with `gofmt`. Embedded assets/browser tests checked with
  pinned Prettier 3.6.2.
- GitHub workflow validated with actionlint v1.7.7. Existing all-push/PR/tag test
  coverage and release dependency gates remain; browser and PostgreSQL coverage
  are added without path skips.
- Playwright 1.62.1 with local Edge: eight browser journeys passed, exercising creation,
  composition/export, references, file and PostgreSQL generation/character reuse,
  failures/conflicts/locks/missing media, cancellation/resumption, keyboard/narrow
  layout and existing fontless projects. Provider transport is mocked and rejects
  unknown hosts. No paid API generation or personal artwork upload was used.
- Browser runner external-host mode is exercised locally. CI's fresh Go fixture
  launch and installed Chromium path remain to be confirmed by hosted CI.
- Visual review inspected actual full-page desktop and 720px screenshots. The
  desktop has outline, preview and inspector columns; the narrow layout moves
  the inspector below. A clipped narrow composition control was fixed and tested.
  Automated accessibility checks cover WCAG2A/AA contrast and semantic controls;
  they are not a claim of exhaustive assistive-technology certification.

## Race gate and remaining verification

The full local race command crashes inside the Go runtime with SIGSEGV/unexpected
return PC, including when package concurrency is reduced. The same failure was
reproduced with untouched main archived into a disposable container and running
`go test -race -count=1 ./app`. This was not a reported data race, and its precise
runtime/environment cause has not been established. Focused race tests for single-job identity, manual approval, queue isolation and
web export passed. Full local race is **not green**; hosted race checks remain required.
No test, lint rule or CI race gate was suppressed.

Independent review used the repository's `paneltree-code-review` skill. Interim
backend findings were fixed and re-reviewed. Final review identified manual
approval, fontless-page authoring and missing single-job identity; focused red and
green evidence is above. Final independent re-review found no remaining actionable
code findings; its font-documentation correction was applied. #26 remains open
pending normal publication and hosted verification.
