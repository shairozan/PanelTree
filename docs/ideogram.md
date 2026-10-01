# Ideogram generation

Ideogram is an optional hosted leaf renderer. Page composition, imports, pins,
manual overrides, and approvals remain local. Ordinary builds never generate or
bill images. The example profiles use Ideogram 4.5 through its hosted API. Explicit Ideogram
3.0 profiles and frozen jobs remain supported; model selection is never automatic.

Powered by Ideogram. Users must follow the [Ideogram Usage Policy](https://ideogram.ai/legal/usage-policy/).

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

Supported 4.5 settings:

- `renderer: ideogram`, `model: ideogram-4-5`.
- `operation: generate` for text-only artwork; `edit` for one source plus up to
  four supporting images. Both use `/v2/image/generate/ideogram-4-5`.
- `quality: very_low | low | medium | high`. `very_low` requires edit.
  Omission uses the provider default (medium for edits, high for text generation).
- `size: source | auto`. Source preserves the source dimensions subject to provider
  limits and requires edit; auto lets the provider choose, typically at 2K cost.
  Exact custom dimensions and masks are not implemented in this integration.
- `magic-prompt: off | on | auto`. Even off can be transformed into the provider's
  structured editing prompt; it does not promise literal prompt execution.
- `negative-prompt`, `rendering-speed` and `style-type` are rejected for 4.5.
- Provider `api-key-env`, `concurrency` (1–8, default 2), `timeout` (default 5m).
  Concurrency is per process; separate CLI processes have separate limits.

Legacy 3.0 profiles use `model: ideogram-3`, `operation: generate | character`,
`rendering-speed: turbo | default | quality`, `magic-prompt: off | on | auto`, and
`style-type: auto | realistic | fiction`. They do not accept quality or size.
Their previous character/style transport and aspect-ratio mapping are retained.

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
traverse symlinks. For 4.5, `--character-image` is the source being edited, sent first as `images`.
Repeat `--style-image` for supporting images, sent afterwards in flag order using
the same `images` field. These are app role names, not separate 4.5 API controls;
explain how to use each reference in the prompt. The source can be a scene as well
as a character. Use edit whenever supplying images. Seed range is
0–2147483647. Inputs are PNG, up to 25 MiB each, 32 MiB combined, 8192 per dimension,
and four megapixels each. 4.5 accepts one source plus up to four supporting images, each with aspect ratio
between 1:6 and 6:1. Legacy 3.0 accepts up to ten style images. The application does
not promise separate identity binding for multiple characters.

Requests queue without contacting Ideogram. Use the returned job ID:

```text
paneltree jobs estimate <project.yaml> --id <job-id> --config <runtime.yaml>
paneltree jobs run <project.yaml> --workers 2 --config <runtime.yaml>
paneltree jobs status <project.yaml> --id <job-id> --config <runtime.yaml>
paneltree asset select <project.yaml> --id <job-id> --revision <revision>
```

`estimate` sends the frozen request with `dry_run=true` and returns the provider's
JSON quote unchanged. It is not a reservation or dollar cap. Automatic spend caps
are not implemented. Live quotes have included `usd_micros`, quantity and a
qualifier; inspect the returned quote before running. Unknown configuration fields,
including `max-cost-usd`, are rejected. `jobs run` drains all queued jobs, so review
that queue before running a paid provider.

For 4.5, the profile's size setting controls output sizing; the leaf's aspect
ratio is not converted to a 3.0 API parameter. Legacy 3.0 freezes its nearest
supported aspect ratio. Provider output resolution
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

Each job saves its resolved profile, prompt, seed, image bytes/hashes, ordered image roles, quality and size (4.5) or aspect
ratio (3.0). Later profile changes do not alter queued requests. Provider completion
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

Automated tests use local HTTP fixtures and do not spend API credits. Direct live 4.5 API experiments produced useful Patrick profile and laughing/
pointing images before this integration change. Those experiments do not replace
the integration fixture tests or verify every account, setting and output host.
Test Patrick's side/rear views, a close-up and an action scene before making this
the default production renderer. Fixed seeds and stored recipes provide provenance,
not guaranteed identical output across provider model updates.

Official contracts consulted:
- https://developer.ideogram.ai/api-reference/images/generate/ideogram-3
- https://developer.ideogram.ai/api-reference/images/generate/ideogram-3-character
- https://developer.ideogram.ai/api-reference/generations/get-generation
- https://docs.ideogram.ai/plans-and-pricing/ideogram-api

## Commercial use and ownership

Checked against the published terms on 2026-10-01. This integration uses the hosted
API, not downloaded Ideogram weights. [Terms §2.1](https://ideogram.ai/legal/tos/)
say Ideogram claims no ownership of input/output, allows commercial use, and
assigns any output rights it acquires to the user. This is a contractual allocation
between the parties, not a guarantee of copyright protection or exclusivity.
Users remain responsible for rights in their inputs and third-party rights.
Section 4.1 grants Ideogram a service license to content; retained ownership does
not mean no license is granted to the provider.

[API terms §2.2–2.3](https://ideogram.ai/legal/api-tos/) incorporate the terms and
require application attribution and a usage-policy link. API inputs/outputs are
excluded from model training except content flagged for usage-policy violations.
Renderer discovery exposes provider attribution and policy; job provenance names
the provider and model. Any future visual UI must visibly identify generated
results and display Ideogram-approved branding on pages offering the model.
Do not treat metadata alone as fulfilling those future UI obligations.
Account-specific negotiated terms may differ. This is an engineering reading of
published terms, not legal advice or a guarantee of copyright enforceability.

## Planned anime checkpoint process

The aim is usable manga building blocks: recognizable characters, readable acting,
and coherent style, with human selection and cleanup. Variation is expected.

1. Select a specific local checkpoint/version and any LoRAs. Review their licenses
   separately for commercial outputs and the intended hosted/local use. Record
   source URL, version/hash, license URL and review date. A family name alone is
   insufficient approval; no checkpoint is approved by this plan.
2. Generate the base artwork in ComfyUI with that checkpoint. Record the workflow,
   prompt, model and LoRA hashes and settings. Ideogram does not load these weights.
3. Let the user select the full-body front design and detailed head anchor. Preserve
   original files and import approved images into the project with license and
   attribution information. Existing reference-set import/approval is the starting
   point; automatic checkpoint orchestration is future work.
4. Use the approved image as the 4.5 edit source, with up to four supporting images
   where useful. Request views, expressions and actions as separate candidates.
   Return to the approved anchor when subsequent edits drift.
5. Select useful candidates, retain provenance/parent lineage, and compose panels.
   Evaluate Patrick's profile, rear, head close-up and pointing/laughing pose for
   recognizability, style, readable action and cleanup effort rather than identical
   detail reproduction.

Follow-up acceptance criteria: checkpoint licenses and exact versions recorded;
base image selection explicit; original anchors immutable; local generation and
4.5 jobs independently inspectable; rejected candidates never replace approved
artwork; manual corrections and pins survive regeneration. AAM XL AnimeMix and
Illustrious XL are candidates for evaluation, not dependencies or license-cleared
recommendations. Verify architecture and license before choosing any other family.

4.5 API contract: https://developer.ideogram.ai/api-reference/images/generate/ideogram-4-5
