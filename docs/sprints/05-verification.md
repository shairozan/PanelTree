# Sprint 05 verification

## Red–green evidence

- Cache publication tests first failed against the stub with `cache publication not implemented`; immutable blob/recipe publication, integrity verification, recovery and interruption handling made them pass.
- `TestIncrementalBuild` first failed because a cold build reported no production/composition work. Connecting the graph and cache produced cold work, zero-work warm builds, placement-only invalidation and lettering-only rerendering.
- Explicit draft YAML first failed with `unknown field "draft"`. Adding the backend-neutral revision/seed values made the test pass; changing either invalidates shared production without automatically changing static pixels.
- Exact premultiplied-pixel testing failed with `{1 2 3 7}` becoming `{0 2 3 7}` through PNG storage. A lossless RGBA envelope fixed cached intermediates.
- Cached/uncached composition comparison failed with a one-unit blue-channel difference. Consistent isolated grouping fixed the discrepancy.
- Independent review found a print-size memory regression. A 4096×4096 row/panel/leaf test reproduced `composition exceeds 256 MiB surface budget`. Reusing completed first-child surfaces fixed it while retaining the budget.

## Acceptance coverage

App tests cover unchanged builds, comments, movement, dialogue, background-only export changes, draft revision/seed, shared cache relocation and renamed byte-identical assets, masks, and cached versus uncached pixels. Build graph tests cover shared sources across pages, an unrelated page remaining cached, changed font bytes, a per-kind renderer version change, and corruption recovery from valid descendants. Store tests cover canonical map ordering, separate recipe/content identities, immutable conflicts, corrupt blobs, interrupted publication before the recipe, concurrent publication and cancellation. Existing protected-output, bundle relocation and rendering regression tests continue to pass.

Actual CLI demo at 240×360: cold build **7 leaf renders, 26 recompositions, 1 export encode**; warm build **0, 0, 0**. Both builds had identical build IDs and byte-identical PNGs. Local generated evidence is in ignored `bin/sprint-05-cold.json`, `bin/sprint-05-warm.json` and `bin/sprint-05-demo/`.

## Final checks and review

Formatting, `go test ./... -count=1`, `go test -race ./...`, `go vet ./...`, `golangci-lint run` and `git diff --check` passed. Lint reported zero issues. These checks ran locally on Windows; hosted CI has not yet run for these unpushed changes.

The independent reviewer invoked `.agents/skills/paneltree-code-review/SKILL.md`, reported the print-size regression, and re-reviewed the surface-reuse correction, draft controls, dependency invalidation, exact RGBA caching, publication and documentation. No actionable findings remained. The reviewer independently confirmed the original 4096×4096 reproduction now succeeds.

## Limits

The default service caches built-in adapters only. It still validates, snapshots and measures on warm builds. Composition blobs use full output surfaces; cache space is not automatically reclaimed. Integrity validation visits reused nodes, not every descendant behind a cached ancestor. Bundles regenerate editable SVG and metadata. Per-page builds share dependencies but there is no automatic book-wide build scheduler. Approval pins, distributed execution and generation remain outside this sprint. See `docs/incremental-builds.md` for the contracts and recovery behavior.

Implementation remains local for user review and push. Sprint 05 is not closed; hosted CI and ticket closure follow the push.
