# Local manga workspace

Build PanelTree with Go 1.27.1. Browser assets are embedded in the executable;
Node.js is only needed for development tests.

```powershell
go build -o bin/paneltree.exe ./cmd/paneltree
New-Item -ItemType Directory -Force -Path .\stories
.\bin\paneltree.exe serve --root .\stories --config .\runtime.yaml
```

Omit `--config` for a filesystem workspace with the built-in renderer. Open the
printed URL, normally `http://127.0.0.1:8910`. Use that exact host; `localhost`
is a different origin. `--listen 127.0.0.1:0` chooses an available port. Only
loopback IPs are accepted. Stop with Ctrl+C.

Repeat `--root` for existing project directories. Alternatively put absolute
`web-roots` paths in host runtime YAML. Roots must exist. The file registry and
preview cache live in the OS user configuration directory under `PanelTree/web`;
`--state-dir` overrides it. They are not exported into stories. Keep this directory
outside the story roots. The server retains at most 128 preview/export URLs per
session; download exports you want to keep. Old session URLs are not restored.

For PostgreSQL, follow [database setup](postgres.md), configure the same durable
blob root used by other clients and run `storage migrate` before `serve`. Database
projects appear automatically. File projects are registered explicitly using their
owning `project.yaml` inside an allowed root. Both use the same application services.
This is a trusted local, single-operator interface, without LAN hosting or accounts.

## Put together a page

1. Create or open a story and choose its storage backend. Follow the ordered book,
   chapter and page outline, then select a panel and a leaf layer.
2. In **Artwork & render**, import a PNG and record its license and attribution.
   The import preserves its bytes and does not select it automatically. Choose
   **Use selected original** to make an explicit manual override.
3. **Add panel**, **Add artwork layer** and **Add lettering** create building
   blank artwork or text using the bundled licensed font. Use the outline to select them.
   Properties provide lettering, normalized frame coordinates, scale and rotation;
   separate controls reorder panels and layers. Invalid geometry is rejected by Go.
4. Refresh the preview or export a PNG at the page's authoring dimensions. Neither
   action invokes a generative renderer. Editable SVG bundles remain available
   through the existing CLI `build --bundle` command.
5. Send a selection to review, then approve it. Approving a selected candidate
   preserves that candidate's pixels. Artwork, placement and full locks use the
   same policy as CLI/MCP. Clear an approved/manual selection explicitly before
   replacing it with a candidate.

Undo/redo covers the last 50 document edits in the current server session.
Approval, selection and lock changes clear that history; they require deliberate
follow-up actions. A newer external edit causes a conflict instead of an overwrite.
Refresh the project, review the external result and reapply your intended change.
History is not persisted across server restarts. Originals and published versions
remain durable. Missing-image messages identify that artwork is unavailable and
provide a refresh action; restore the source when a project asset is missing.

## Reuse a character and its references

Select an imported original, open **Character library**, and create an immutable
character package with an ID, version and description. **Use authored character**
applies it locally. With PostgreSQL configured, **Publish to shared library** makes
that version discoverable by every story using the same database/blob root.

Search published characters, inspect original thumbnails, license metadata and
approved directional cards, then choose **Use pinned version** for the selected
layer. The inspector displays the pinned package and reference version. Other
published versions are marked explicitly; they never upgrade a story automatically.
Costume, expression and pose fields select IDs defined by that package and remain
local to the story. The initial package form creates a description and front
reference; richer package states can still be authored through the existing CLI/files.

File-backed stories receive immutable portable copies plus a library-version
manifest; later PostgreSQL import preserves those bindings. New publication
versions leave old story choices and manual artwork intact. Use a new package
version to change a design.

**Character references** opens the reference workshop. Create/open a set by ID,
import the front into `body/front`, and explicitly accept it. Approve the head/front
and remaining body/head slots in dependency order. You can instead prepare a
reference candidate with a configured profile and approved parent anchor. Review
its estimate in the artwork tab, run it, and **Collect reference candidate** into
the open set. Accept/reject decisions are separate from generation.

The workshop shows parent-lineage status. If a parent changes, affected approvals
are invalidated. Regenerate or explicitly reapprove against current parents. After
all eight views are current and accepted, publish a named reference version. It
creates four body/head directional cards. Supply that version when publishing the
character to share those approved references. Published sets cannot be overwritten.

## Generate deliberately

Use [Ideogram runtime profiles](ideogram.md) to configure a model, operation,
quality and size on the server. API keys stay in the server environment. The UI
selects named profiles from capabilities; it cannot change credentials or point a
renderer at an arbitrary provider URL. Missing credentials disable that profile.

The anime workflow starts with a local checkpoint image (for example an image
created in ComfyUI), imported as an original. Choose it as the **source image**,
then add and order supporting references. Hosted Ideogram 4.5 edits receive the
source first. Checkpoints are not uploaded or loaded into Ideogram. The UI shows
configured quality/size and image-role limits; 4.5 `size: source/auto` controls
provider output size, not the numeric composition target dimensions.

**Prepare & request estimate** freezes a job and requests a dry-run quote. A quote
is not a spend cap. A visible quote-unavailable response can also be reviewed.
**Confirm & run this candidate** starts only that approved job, not other queued
jobs. Jobs show progress, failures and launch conflicts. Side-by-side review uses
the frozen input pixels. **Select candidate** is a separate revision-checked edit;
**Reject candidate** keeps the result for inspection but prevents future selection.
Rejection does not undo any selection already made.

Cancel requests stop local work when possible; provider work or billing may
continue. **Resume known execution** obtains a new confirmation and resumes a
recorded provider execution without resubmitting it. Unknown submission outcomes
are not automatically retried. Restarting the server loses confirmation tickets;
review the estimate again. CLI `jobs run` still drains its queue: coordinate that
separate explicit action with browser work.

Powered by Ideogram attribution and the usage-policy link appear beside generation.
User-supplied license metadata, provider terms and any local checkpoint license all
need review. The UI does not grant commercial-use clearance; the broader audit
remains tracked in #21.

## Browser checks

From `web`, use Node 24, `npm ci --ignore-scripts`, then
`npx playwright install --with-deps chromium` and `npm test`. Set
`PANELTREE_TEST_POSTGRES` to a disposable PostgreSQL 18 database; the fixture creates
an isolated schema. Its mocked transport blocks every unrecognized outbound
request, uses no real API key, and incurs no generation charges. The runner starts
and stops the Go fixture. To use an already-running fixture, set
`PANELTREE_BROWSER_URL`; `PANELTREE_BROWSER_CHANNEL=msedge` selects installed Edge.
Do not run these tests against a real workspace server.

`npm run format:check` checks the embedded assets and browser tests. CI runs the
browser journey on every push, PR and release validation, alongside the existing
Go checks. Screenshots are saved under `web/test-results`. See the
[verification record](sprints/26-verification.md) for test coverage and limitations.

New artwork layers start with a transparent PNG. Lettering uses the bundled Go
Regular font; its license is copied with the immutable font into the project, so
image-only projects also support composition without a pre-existing text layer.
