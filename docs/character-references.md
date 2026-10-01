# Approved character references

Character packages describe a design. Published reference sets add actual image
conditioning. This improves the available inputs; it does not guarantee that a
model will preserve identity, follow a pose, or understand a reference sheet.

Each set retains eight originals: `body/front`, `body/left`, `body/right`,
`body/rear`, and the corresponding `head/...` slots. Left and right mean the
character's anatomical side visible to the camera. No view is made by mirroring.
Unseen features remain proposed designs until reviewed.

## Create and review

Use `paneltree character-reference project.yaml --set alex --action create`.
The result contains a reference `revision`, independent of the project's editorial
revision. Pass that value as `--revision` for every subsequent mutation. Inspect
and resume with `--action inspect`; no mutation occurs on inspection.

Start with a text-generation runtime profile:

```powershell
paneltree --config text-runtime.yaml character-reference project.yaml --set alex --action request --revision REV --slot body/front --key alex-front-1 --prompt "short black hair, blue coat, red patch on the left sleeve" --seed 17 --width 512 --height 768 --license "CreativeML Open RAIL-M" --attribution "SD 1.5; original character design"
paneltree --config text-runtime.yaml jobs run project.yaml --workers 1
paneltree character-reference project.yaml --set alex --action collect --revision REV --job-id JOB_ID
```

Use the updated revision from each response. A request returns its durable job
ID in `jobs[KEY]`. Repeating an identical request with its original revision and
key returns the same job. A changed request needs a new key. Existing `jobs
status`, `jobs cancel`, and `jobs run` commands also apply to these jobs.

Collected candidates are never automatically accepted. View the candidate's
`image` at `.paneltree/assets/IMAGE_HASH.png`, then explicitly accept or reject:

```powershell
paneltree character-reference project.yaml --set alex --action accept --revision REV --slot body/front --candidate CANDIDATE_ID
```

The standard front prompt requests a full figure, arms at sides, neutral
expression, plain background and even lighting. Review that those requirements
actually appear before accepting. To import existing or externally cropped
artwork, use `--action import --slot SLOT --path assets/front.png --license ...
--attribution ...`, then accept separately. Imported files must be PNGs inside
the project; originals are preserved in the immutable asset store.

Switch to an image-enabled runtime profile for derived views. The included
`examples/comfyui/sd15-reference-profile.json` uses core ComfyUI `LoadImage`,
`ImageScale`, `VAEEncode`, and `KSampler` nodes with denoise 0.65. Update its
checkpoint filename, model hash and backend identity to match your installation.
Use a separate runtime config pointing at this profile. It requires exactly one
image with role `reference`; the text profile cannot consume images.

Request and review `head/front` from the accepted figure, then the three other
body views, then their head details. The latter use a deterministic row containing
the accepted front head and corresponding body view. Front head is an
image-conditioned refinement of the figure, not automatic face detection or an
automatically approved crop. An externally prepared crop can be imported instead.
All derived requests use the same request/run/collect/accept commands. Additional
prompt details can describe corrections. Each retry uses a fresh job key.

Inspect `accepted` to see which slots remain missing. Candidates keep their
original parent IDs and `approved_against` lineage. Replacing an accepted parent
clears dependent approvals, including transitive descendants. An older job may
still finish and be inspected, but acceptance fails unless explicitly reviewed
with `--reapprove`. That records the current parent lineage without erasing the
original generation inputs. Accepted siblings are preserved. Rejected imports
can be imported again to create a new reviewable attempt.

## Publish and reuse

Once all eight views are accepted, explicitly publish:

```powershell
paneltree character-reference project.yaml --set alex --action publish --revision REV --version design-1
```

Publication atomically records all accepted originals and four body/head cards.
`directional-card/v1` uses two 512px square cells, body then head, with white
letterboxing and preserved aspect ratios. The originals remain available at
their original resolution. Publication is rejected if any approval is missing
or stale. A published version cannot be overwritten. Further edits are proposals
for a new version; publishing that version does not change prior versions or
approved panel artwork. Back up `.paneltree/references` and `.paneltree/assets`
alongside the project; these are durable source assets, not disposable caches.

Attach an explicit version to a layer's existing character package:

```yaml
character:
  package: characters/alex.json
  reference_set:
    set: alex
    version: design-1
    directions: [front, left, right, rear]
    packing: cards-row/v1
```

`cards-row/v1` packs the requested directional cards in the listed order into one
image with role `reference`. `directional-cards/v1` sends separate cards with
roles `card/front`, `card/left`, etc. Choose the latter only with a runtime profile
declaring exactly those ordered image bindings. Count, roles, format, input hashes
and size limits are validated before backend submission. No references are
silently discarded and no text-only fallback is selected automatically.

Use the existing `asset request`, `jobs run`, and candidate selection workflow.
Panels reuse published artwork without generating the references again. Frozen
recipes include reference bytes and hashes, selection/packing policy, profile
settings, model identities and the caller's prompt/seed. Job provenance exposes
consumed hashes and roles without requiring an inference from the prompt.

`renderer_list` / `paneltree renderers` report image roles, PNG support, maximum
four megapixels per input, 32 MiB total input bytes and upload retention. Runtime
bindings must connect the `LoadImage` IMAGE socket to the output graph. This is
structural validation; a custom node can still ignore inputs or use them poorly.
Validate every new model/profile visually. The SD 1.5 example is basic img2img,
not an identity adapter: contact sheets can leak into the output and pose changes
can fail. Stronger reference-aware workflows require their own verified profiles.

ComfyUI receives multipart uploads under `input/paneltree/HASH.png`. Uploads are
content addressed and reused; they are retained because ComfyUI has no verified
scoped deletion contract here. Administrators may remove this backend staging
directory after dependent jobs finish, never the project's canonical asset store.

MCP exposes the same lifecycle via `character_reference`, accepting the JSON
fields corresponding to these flags (`project_file`, `set`, `action`, `revision`,
`slot`, `generation`, `key`, etc.). Normal allowed-root and workspace protections
apply. Imported licensing/attribution, generated job recipes and parent lineage
are retained for the future #21 licensing work; these fields record provenance
without automatically deciding whether a license permits a particular use.

Visual review UI remains future #13/#14 work. The shared service and CLI/MCP
workflow are available independently.
