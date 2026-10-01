# Character packages

A character package keeps identity descriptions, palettes, versioned artwork,
costumes, expressions, poses and exact prop references in one backend-neutral
file. Multiple panel sources refer to it without copying its descriptive data.
See the complete three-panel project in `examples/characters/`.

Add a `character` reference to the existing leaf source in page YAML:

```yaml
source:
  kind: image
  path: ../assets/hero.png
  character:
    package: characters/alex.json
    costume: coat
    expression: calm
    pose: front
    props: [lantern]
```

The original source remains the static fallback. Unlike the source image path,
the package path and every reference path inside the package are relative to
the owning project directory, not the page or package directory. Absolute paths,
traversal outside the project, and symlinks are rejected during resolution.

Packages use strict JSON and schema `paneltree/character/v1`. Unknown fields,
duplicate object keys, explicit null, numeric versions, and multiple documents
are rejected. A package requires `id`, string `version`, and `description`.
Optional `palette` entries are six-digit hex colors. Each costume, expression
and pose requires its own `id`, string `version`, and `description`; it may also
include `references`. States are optional but explicitly selected IDs must exist.

Artwork references and props require `id`, string `version`, `path`, `license`,
`attribution`, and `description`. The resolver records SHA-256 of each selected
file. IDs must be unique in each state category, in props, and across the selected
artwork reference set. License text is author-supplied provenance, not automated
permission verification. The example's geometric reference assets retain their
CC0 notice; it does not relicense model weights or arbitrary imported artwork.
Package files are limited to 1 MiB and each reference file to 32 MiB.

## Generation and capabilities

Use the normal `asset request` command or MCP `asset_request` on a leaf carrying
the reference. There are no renderer-specific fields in the package or page.
CLI, MCP and application callers resolve the same dependencies through the
shared service. Endpoint and workflow configuration remain in runtime YAML.

The ComfyUI adapter appends the resolved identity, selected costume/expression/
pose descriptions, palette and selected prop descriptions to the caller's
prompt. Its frozen workflow and job fingerprint include that selected package
content and reference hashes. Prompt length after expansion is bounded.

`renderers` / `renderer_list` lists character capabilities and limitations.
Job responses retain the resolved package, selected state versions, reference
paths/hashes/licenses/attribution and limitations in `generation_provenance`.
`character_status` is `current`, `changed`, or `unavailable`; an unavailable
dependency also includes `character_diagnostic`. Status observations do not
mutate jobs or selections. The stored provenance always describes the original
request, even when the current package is missing or changed.

The current adapter supports description conditioning only. Reference artwork
is hashed for provenance and invalidation but its pixels are **not** uploaded
or used as conditioning. Exact prop references and poses provide descriptions,
not exact geometry or pose control. Identity consistency is not guaranteed.
There is no automatic matting: outputs remain opaque RGB. Use whole-panel
portraits/backgrounds for this evaluation, not isolated character layers.

## Changes and preserved selections

Editing selected costume/reference versions, descriptions, license metadata or
reference bytes invalidates dependent drafts. Reference byte changes are
detected even without a version bump. Unselected costumes/poses/expressions and
unselected props are omitted from the fingerprint, so valid edits to them do
not invalidate unrelated drafts. Package identity/version/description/palette
and common references affect all users of that package.

Package files are external dependencies, so their changes do not increment the
authored project revision. Current dependency hashes are checked before queued
character work executes and again when selecting a candidate. Changed queued
work fails without submission to the renderer; already completed jobs keep their
artifact and provenance but cannot be selected against changed dependencies.
Changes after the execution check do not interrupt an in-flight render; the
selection check still protects the result. Use a new idempotency key for a new
draft. Reusing a key returns the original job, not a newly resolved request.

Approved pins, manual overrides and scoped locks retain their existing rules.
Changing a package never automatically changes a displayed selection. Already
selected artwork can still be built with no package or backend present. The
existing project-revision gate remains in force: authoring edits or selection
transactions can stale other candidates independently of package dependencies.
Static renders do not consume package contents. Portable image/SVG exports
preserve rendered artwork; retain the original project and packages to generate
new character candidates.

## Reproducible visual evaluation

Configure the SD 1.5 profile as in [ComfyUI setup](comfyui.md). The fixture uses
the same character and costume in front, three-quarter and side-view panels.
It requests three 512-square RGB images with seed 17, then explicitly selects
each candidate and builds the 1536 x 512 evaluation strip. The current project
revision is reread before each request/selection sequence.

```powershell
$env:PANELTREE_COMFY_URL = 'http://127.0.0.1:8188'
$env:PANELTREE_COMFY_PROFILE = 'C:\runtime\sd15-profile.json'
$env:PANELTREE_CHARACTER_SMOKE_DIR = 'C:\books\new-character-evaluation'
go test ./app -run '^TestCharacterRealBackendEvaluation$' -count=1 -v -timeout 17m
```

The destination must not exist. The test copies the example project there and
retains the rendered strip and frozen job provenance. A profile supporting
`negative_prompt` is required. In Docker, mount the repository at `/workspace`,
use paths within that mount, and reach the host backend at
`http://host.docker.internal:8188`. Unset the opt-in variables afterward.
Ordinary CI skips this test and uses fake-server tests without a GPU.

For each panel, compare against the reference design and the other panels.
Record **0 = absent/wrong**, **1 = partial/uncertain**, **2 = matches** for:

| Dimension | Inspect |
| --- | --- |
| Identity | Face shape, hair silhouette/fringe and stable distinguishing details |
| Costume/palette | Garment shape, orange coat, navy trousers and black boots |
| Expression/pose | Neutral expression and requested front/turn/side orientation |
| Prop fidelity | Lantern presence, rectangular geometry and circular light |
| Composition | Full body, one person, panel fit and no unintended cropping |

Report specific failures as well as scores, including details that are hidden
and cannot be judged. This is a manual comparison rubric, not an automatic score
or proof of identity. Record model/profile hashes, seed, backend version and
hardware. See [Sprint 11 results](sprints/11-verification.md) for the actual
baseline, including its failures.
