# Sprint 06 verification

Implementation for GitHub issue #6 remains local and uncommitted. The issue is
open; no branch, PR, release, or tag was published. Hosted checks are unverified.

## Red–green evidence

- Workspace conflict and whole-changeset validation tests first failed with
  `workspace transactions not implemented`. They pass with serialized expected-
  revision checks, staged validation and journaled publication.
- Approval, separate lock scopes and manual override tests first failed with
  `editorial edits not implemented`. They now pass through the shared edit service.
- The CLI test first failed with `unknown flag: --revision`; command factories
  now route revision-aware editorial operations through the service.
- Group-descendant placement/selection, externally replaced locked sources, and
  authoring-source preservation each failed against the first implementation,
  then passed after corrections.
- Independent review findings were reproduced before fixes: placement-only
  selection was rejected; manual/mask replacements bypassed locks; `.yml` edits
  poisoned recovery; external deletion was ignored; lock/selection ordering
  committed inconsistent state; child unlock lost ancestor selection inputs;
  standalone root deletion bypassed checks; and reparenting selected artwork
  escaped an asset-locked group. Each has a passing regression test.

Dependency-download permissions, temporary-executable application-control errors,
and sandbox fixture-access failures were environment failures, not red evidence.
Tests subsequently executed successfully with the relocated Go installation at
`C:\Program Files\Go` and the required filesystem permissions.

## Acceptance coverage

Workspace tests verify concurrent same-revision conflict without lost edits,
whole-graph validation before writes, roll-forward recovery before and during
partial publication, cancellation preserving the old revision, cancellable lock
waiting, and `.yml` document edits. OS-backed locks support Windows and Unix.

Application tests verify draft/review/approval transitions, persistent pins,
stale source provenance, distinct rendered approved artwork surviving request
changes and cache deletion, corrupt-pin failure, missing-pin failure with zero
rasterizer calls, unchanged manual files, independent asset/placement locks,
ancestor and descendant protection, mask/source/manual-byte replacement,
deletion, selection reparenting, explicit unlocks, order-independent multi-operation
changesets, standalone-root enforcement and unchanged authoring inspection.
CLI integration tests verify ordinary lock means all, stale revisions fail and
explicit unlock succeeds at the current revision.

## Final checks and independent review

- `gofmt` applied to Go sources. Existing CRLF checkout formatting was normalized
  where needed by the pinned linter; this introduces no unrelated semantic edits.
- `go test ./... -count=1`: passed.
- `go vet ./...`: passed.
- `golangci-lint run`, CI-pinned **v2.14.0**: passed, **0 issues**.
- `git diff --check`: passed.
- Workspace tests cross-compiled for Linux and macOS; they were not executed on
  those platforms locally.
- `go test -race ./...`: could not run because local CGO is disabled and no C
  compiler was available. The existing hosted Linux race job remains required.

The independent agent used `.agents/skills/paneltree-code-review/SKILL.md`, reviewed
the actual tracked and untracked implementation, requirements, tests and docs,
and re-reviewed substantive corrections. Final result: **no actionable findings
remain**. Its own runtime attempt was blocked by Windows fixture access, so the
runtime evidence above comes from the implementing agent's successful checks.

## Remaining limits

See [editing documentation](../editing.md). Transactions cover participating
local services and process interruption, not concurrent external editors or
power-loss durability. Existing-document replacement is supported; creation of
new document files is not. Placement protection is conservative. Source
provenance is separate from output/placement choices. Manual override paths are
absolute and require reselection after relocation. Builds serialize edits through
export. Keep protected `.paneltree/assets` and editorial state in project backups.
