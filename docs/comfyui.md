# ComfyUI draft generation

ComfyUI is an optional leaf renderer. It turns an explicit semantic prompt and
seed into one RGB draft, sized from the existing resolved leaf. Requesting or
running a job never changes the current selection. Selection, approval, manual
overrides and scoped locks use the same services as static assets. Page and
layout YAML remain backend-neutral and unchanged.

## Configure a backend

Run your own trusted ComfyUI server. Copy `examples/comfyui/` into a local runtime
configuration directory. In `sd15-profile.json`, replace the checkpoint filename,
checkpoint SHA-256 and backend identity placeholders. Record the ComfyUI commit,
custom-node versions and **all** model/resource hashes consumed by your workflow.
Use a checkpoint whose license permits your intended use. No model is bundled
or downloaded by PanelTree. The example is a conventional SD 1.5 text-to-image
workflow, not a claim of compatibility with every checkpoint.

The profile is a strict JSON document, separate from story YAML:

- `version` is `comfy-profile/v1`; `name` and `revision` identify your profile.
- `backend_identity` identifies the runtime and custom-node environment.
- `models` maps meaningful dependency names to immutable identities/hashes.
- `workflow` is an API-format graph (`class_type` and `inputs`), not a UI workflow.
- `bindings` map `prompt`, `seed`, `width`, `height`, and optionally
  `negative_prompt` to existing node inputs. They must be distinct. Bound values
  replace workflow defaults; an omitted negative prompt binds an empty string.
- `output_node` identifies the node producing exactly one PNG. Use batch size 1;
  multiple images are rejected instead of choosing one arbitrarily.

`comfyui-url` and `comfyui-profile` must be supplied together in runtime YAML.
Relative profile paths resolve beside that runtime file. Configuration is local
to each command invocation. MCP takes the same runtime file at server startup;
clients cannot supply endpoint URLs or workflow/profile paths in tool calls.

```yaml
log-level: info
comfyui-url: http://127.0.0.1:8188
comfyui-profile: sd15-profile.json
```

`paneltree renderers --config runtime.yaml` advertises a configured ComfyUI
adapter with `output_kinds: ["rgb"]`. Available means configured, not a health
check. Before submission the adapter queries `/object_info` and rejects missing
node classes. ComfyUI performs full graph/model validation on submission. Remote
model bytes and backend versions cannot be verified by this API: recorded
identities are operator assertions. Update them whenever the environment changes.
URLs containing credentials, queries or fragments and HTTP redirects are rejected.

## Request and explicitly select

Get `REVISION` from `paneltree inspect my-book/project.yaml`. Use a background
leaf for RGB generation; this sprint does not produce isolated characters.

```sh
paneltree asset request my-book/project.yaml --config runtime.yaml --renderer comfyui --revision REVISION --key setting-draft-1 --page page-01 --panel p1 --layer setting --prompt "moonlit coastal city, comic book background" --seed 17 --width 800 --height 1200
paneltree jobs run my-book/project.yaml --config runtime.yaml --workers 1
paneltree jobs status my-book/project.yaml --id JOB_ID
paneltree asset select my-book/project.yaml --config runtime.yaml --revision REVISION --id JOB_ID
paneltree build my-book/project.yaml --page page-01 --output generated.png --width 800 --height 1200
```

Page output dimensions determine the leaf's generation dimensions, rounded up
to multiples of eight (maximum 8192 per axis and 4,194,304 pixels). Normal
composition fits those pixels into the unchanged frame. Output must match the
requested dimensions. PNG responses are bounded to 32 MiB before decoding.
Unexpected alpha is flattened against white; it is never advertised as isolated
RGBA. `--output-kind isolated-rgba` is rejected. Matting, consistency guarantees,
image-reference uploads and automatic prompt synthesis are outside this sprint.

For another candidate, use a new key and seed. Each job is independently
selectable while its revision is current; selecting one changes the revision,
so other candidates from the older revision then become stale. Requests may be
made for approved, manually overridden or locked leaves, but their selections
remain protected. Explicitly clear protected selections/unlock as appropriate
and submit at the resulting revision before choosing a replacement.

MCP uses `asset_request` with the usual project/revision/key/target fields plus:

```json
{"renderer":"comfyui","generation":{"prompt":"moonlit coastal city","seed":17,"output":"rgb"}}
```

Use `jobs_run`, `jobs_status`, and `asset_select` as documented in [MCP](mcp.md).
All command/server processes that submit, execute or select must use the same
effective profile. Status, listing and cancellation work without loading it.

## Provenance, retries and cancellation

Job status includes `generation_provenance`: adapter version, backend identity,
profile name/revision/hash, model identities, exact seed, RGB capability and
dimensions. The integrity-checked `input.json` additionally retains prompts,
the full profile, effective workflow and source snapshot. Effective profile
content and model identities participate in the job input hash/fingerprint.
Changing them prevents execution/selection of an older candidate. Already
selected pins and approved artwork remain usable without any backend.

An identical request/key returns the original job, even after completion or
failure; changing its semantic request under that key is an idempotency conflict.
No second `/prompt` is sent. Transient GET transport errors, HTTP 429 and 5xx
responses receive up to two retries. Individual HTTP calls have a 30-second
limit, polling uses 500 milliseconds, and generation has a five-minute deadline.

ComfyUI's `/prompt` API does not guarantee idempotent submission. A failed or
malformed POST response is **not** automatically retried, since the backend may
have accepted it. Check backend history/queue before deliberately retrying under
a new key. Process-death recovery marks interrupted jobs failed and never
resubmits them; backend execution may continue after a process is killed.

Cancellation and failures after a prompt ID is received attempt the atomic,
prompt-scoped `POST /api/jobs/{id}/cancel` endpoint, with a separate three-second
cleanup deadline. Use a ComfyUI build supporting this endpoint. There is no
fallback to global interruption. An older/unreachable server may continue work;
the job diagnostic reports that cancellation could not be confirmed. No candidate
is published after cancellation. An ambiguous submission without a returned ID
cannot be cancelled automatically. These are transport limits, not exactly-once
execution guarantees. Protocol references: [ComfyUI server](https://github.com/comfyanonymous/ComfyUI/blob/master/server.py)
and [API workflow example](https://github.com/comfyanonymous/ComfyUI/blob/master/script_examples/basic_api_example.py).

## Opt-in real-backend smoke test

Ordinary tests use `httptest` and require no GPU or ComfyUI installation. To run
real compute, start the configured backend and set these variables (PowerShell):

```powershell
$env:PANELTREE_COMFY_URL = 'http://127.0.0.1:8188'
$env:PANELTREE_COMFY_PROFILE = 'C:\runtime\sd15-profile.json'
$env:PANELTREE_COMFY_SMOKE_DIR = 'C:\books\new-comfy-smoke'
go test ./app -run '^TestComfyRealBackendSmoke$' -count=1 -v -timeout 7m
```

The destination must not exist. The test initializes the licensed demo, requests
and selects the `page-01/p1/setting` leaf at seed 17, and retains `comfy-smoke.png`
plus job provenance. Review the image for correct panel fit and visible output;
record the profile hash, backend commit, model hashes, hardware and any visual
failures. The test proves transport/composition operation, not artistic quality,
character consistency or identical pixels across hardware. Unset all three
variables afterward to return to GPU-free tests. The repository verification
record explicitly distinguishes fake-server checks from an actual smoke run.
