# Ideogram generation

Ideogram is an optional hosted leaf renderer. Page composition, imports, pins,
manual overrides, and approvals remain local. Ordinary builds never generate or
bill images. The implementation uses the documented v2 Ideogram 3.0 endpoints;
it does not select a newer model automatically.

## Configure

Use `examples/ideogram/runtime.yaml` as a runtime configuration, separate from
strict project YAML. Set `IDEOGRAM_API_KEY` in the environment of the process
running PanelTree (including the MCP host). Ideogram API billing is separate
from its website subscription. Never commit the key or put it in a prompt.

Run `paneltree renderers --config <runtime.yaml>` to inspect configured profiles,
model, operation, availability and image-role limits. Profiles are invocation-local;
configuration does not leak into later commands. Missing credentials mark the
provider unavailable. Unsupported model/operation/settings fail at startup.

`generation.default-profile` applies to generation requests without an explicit
profile or renderer. `--generation-profile` overrides it. An explicitly conflicting
`--renderer` fails. Existing `--renderer comfyui` remains supported. Static asset
requests still default to builtin. ComfyUI rejects the new character/style path
options instead of silently discarding them.

Supported settings:

- `renderer: ideogram`, `model: ideogram-3`.
- `operation: generate` (text and optional style images) or `character` (exactly
  one character image and optional style images).
- `rendering-speed: turbo | default | quality`.
- `magic-prompt: off | on | auto`; off is recommended for controlled comparisons.
- `style-type: auto | realistic | fiction` (use auto with style references).
- Provider `api-key-env`, `concurrency` (1–8, default 2), `timeout` (default 5m).
  Concurrency is per process; separate CLI processes have separate limits.

## Request a candidate

For example, with project-relative PNG files `assets/patrick.png` and
`assets/story-style.png`, supply these options to `paneltree asset request`:

```text
--config <runtime.yaml> --generation-profile story-character
--revision <current-project-revision> --key patrick-side-1
--page page-01 --panel p1 --layer hero
--prompt "Patrick in left profile, full body, arms at his sides"
--seed 42 --character-image assets/patrick.png
--style-image assets/story-style.png
```

The owning project YAML is the positional argument. Obtain the revision with
`paneltree inspect <project.yaml>`. Paths must be inside that project and cannot
traverse symlinks. Repeat `--style-image` for more style references. Seed range is
0–2147483647. Inputs are PNG, up to 25 MiB each, 32 MiB combined, 8192 per dimension,
and four megapixels each. At most ten style images are accepted. Multi-character
identity conditioning is unsupported.

Requests queue without contacting Ideogram. Use the returned job ID:

```text
paneltree jobs estimate <project.yaml> --id <job-id> --config <runtime.yaml>
paneltree jobs run <project.yaml> --workers 2 --config <runtime.yaml>
paneltree jobs status <project.yaml> --id <job-id> --config <runtime.yaml>
paneltree asset select <project.yaml> --id <job-id> --revision <revision>
```

`estimate` sends the frozen request with `dry_run=true` and returns the provider's
JSON quote unchanged. It is not a reservation or dollar cap. Automatic spend caps
are not implemented: the public documentation describes PriceQuote but did not
expose a verifiable schema during development. Unknown configuration fields,
including `max-cost-usd`, are rejected. `jobs run` drains all queued jobs, so review
that queue before running a paid provider.

Generation maps the leaf's aspect ratio to the nearest supported Ideogram ratio;
that exact ratio is frozen and visible in provenance. Provider output resolution
may differ from the page's requested pixel dimensions. Panel composition fits the
saved candidate into the authored layout. This renderer produces RGB candidates;
it does not implement transparent cutouts, matting, remix or inpainting.

## Character reference sets

`character-reference --action request` accepts the same profile, style-image,
seed and prompt options. Use `story-background` to invent the initial front image
without an image input. For derived views, use `story-character`; the approved
parent becomes the character image automatically. When a head view has both a
body and a head parent, supply `--anchor head/front` or the required body slot.
All required parents remain recorded in lineage, even when one is selected for
conditioning. No contact sheet is silently substituted. An external
`--character-image` cannot replace a required approved parent.

Collect, accept/reject and publish remain separate actions. Generated images are
never automatically accepted. Published reference cards are retained, but panel
requests currently require an explicit `--character-image` path rather than
silently selecting or packing a published card. Its bytes and hash are frozen.

## Recovery and provenance

Each job saves its resolved profile, prompt, seed, image bytes/hashes, and aspect
ratio. Later profile changes do not alter queued requests. Provider completion
metadata records returned prompt, seed, actual dimensions and usage cost when the
provider supplies it. API keys and signed image URLs are not saved in recipes or
completion metadata. Saved candidates can be selected without API credentials or
a configured provider, subject to the normal freshness/lock checks.

A remote generation ID is persisted before polling. If the process dies, a job
with a known ID returns to the queue and polls that same generation. After a
polling/download failure or cancellation, use:

```text
paneltree jobs resume <project.yaml> --id <job-id>
paneltree jobs run <project.yaml> --config <runtime.yaml>
```

Resume requires a known remote execution; it never creates a new generation.
Cancellation stops local waiting and publication, not necessarily the provider's
work or billing. A failed/ambiguous submission without a saved remote ID is never
automatically repeated. Inspect the provider dashboard before intentionally
submitting a new key. HTTP error bodies are not echoed; status codes are reported.
Expired provider image URLs may prevent recovery of an old result.

MCP uses the same services: `asset_request.generation.profile`,
`character_reference.generation.profile`, `character_reference.anchor`,
`generation.character_reference`, and `generation.style_references` correspond to
the CLI options. `renderer_list`, `jobs_estimate`, `jobs_resume`, `jobs_run`, and
existing acceptance tools cover the same lifecycle.

## Validation boundary

Automated tests use local HTTP fixtures and do not spend API credits. A live
account smoke test is still needed for account-specific character/style access,
wire compatibility, output hosts, price-quote contents, quality, latency and cost.
Test Patrick's side/rear views, a close-up and an action scene before making this
the default production renderer. Fixed seeds and stored recipes provide provenance,
not guaranteed identical output across provider model updates.

Official contracts consulted:
- https://developer.ideogram.ai/api-reference/images/generate/ideogram-3
- https://developer.ideogram.ai/api-reference/images/generate/ideogram-3-character
- https://developer.ideogram.ai/api-reference/generations/get-generation
- https://docs.ideogram.ai/plans-and-pricing/ideogram-api
