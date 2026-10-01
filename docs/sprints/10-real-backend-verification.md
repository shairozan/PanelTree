# Sprint 10 real-backend verification

The opt-in real-backend smoke test passed against the merged Sprint 10 code at
`77e1b800d269114df3193fbec4ab773247b1c1bb`. This supplements the historical
pre-merge record in `10-verification.md`; no production behavior was changed.

## Environment and recipe

- GPU: NVIDIA GeForce RTX 4090, 24,564 MiB reported by nvidia-smi.
- ComfyUI commit: `8cfe5e1ecb97512dea8deaac15e1228d7e6feeb1`.
- ComfyUI image ID: `sha256:a63bf18f31c7ac7d7748d79a628dd661fb440371ab7e97127aead2e5475e62d8`.
- Container model path: `/basedir/models/checkpoints/v1-5-pruned-emaonly.safetensors`.
- Checkpoint SHA-256, measured inside the container:
  `6ce0161689b3853acaa03779ec93eafe75a02f4ced659bee03f50797806fa2fa`.
- Core-node SD 1.5 profile from `examples/comfyui/sd15-profile.json`, filled with
  the installed filename and identities. No custom nodes are consumed.
- Profile hash: `241eb9ee21e62da5a91c407ac9b84c5d495018fa11073ee5724740436fd0125a`.
- Seed 17, 20 steps, CFG 7, Euler sampler, normal scheduler, batch size 1.
- Prompt: `moonlit coastal city, comic book background, no people`.
  Negative prompt is empty, as specified by the existing smoke test. The user's
  saved UI workflow `sd-image-generation` uses the same checkpoint and sampler,
  but has a negative prompt and a 512-square canvas; it is not the smoke recipe.
- Requested leaf output: 440 x 568 RGB; composed page: 800 x 1200.

The backend checkout reports removed sample/model placeholder files because
the container uses `/basedir` for mutable data. The commit alone is therefore
not a claim of an unmodified runtime. Backend identity also includes the image
ID and core-node-only workflow scope. Model/backend provenance remains an
operator assertion, as described in `../comfyui.md`.

## Execution and result

Windows application control blocked the installed native Go executable. The
test ran with Go 1.27.1 linux/amd64 in `golang:1.27.1`, using a repository bind
mount and `http://host.docker.internal:8188` to reach the existing GPU backend.
The Go container does not require GPU passthrough.

```sh
PANELTREE_COMFY_URL=http://host.docker.internal:8188 \
PANELTREE_COMFY_PROFILE=/workspace/bin/sprint10/sd15-profile.json \
PANELTREE_COMFY_SMOKE_DIR=/workspace/bin/sprint10/smoke \
go test ./app -run '^TestComfyRealBackendSmoke$' -count=1 -v -timeout 7m
```

`TestComfyRealBackendSmoke` passed in 3.68 seconds. It requested a draft,
executed the job, checked success, explicitly selected the candidate and built
the page. Visual inspection confirmed a visible generated city background
inside the first panel, with the original character and caption composited
above it and the other panels still present.

Local artifacts are retained under ignored `bin/sprint10/`: runtime profile,
runtime YAML, smoke log, demo project with durable job/provenance records, and
`smoke/comfy-smoke.png`. They are not included in Git. For a rerun, choose a new
smoke destination because the test requires a nonexistent directory.

This verifies successful real transport, generation, selection and composition.
It does not verify real-backend cancellation or failure recovery, artistic
quality, character consistency, transparency/matting, or identical pixels across
hardware. Failure-path coverage remains in the existing fake-server tests.
No new behavior was implemented, so a new red-phase test was not applicable.
No release, tag, PR or issue update was published for this verification.

## Regression checks

With the smoke environment variables absent, all checks passed in the Go 1.27.1
Linux container: `go test -count=1 ./...`, `go vet ./...`,
`go test -race -count=1 ./...`, and CI-pinned `golangci-lint` v2.14.0
(`0 issues`). Logs are retained in `bin/sprint10/`. These are local Docker runs;
no new hosted CI run or native Windows/macOS validation is claimed.

Independent review using `.agents/skills/paneltree-code-review/SKILL.md` found
no actionable issues. The reviewer inspected the report, smoke log, frozen
recipe, successful job, selected pin, regression logs and composed page. Their
Docker access was denied by the sandbox, so toolchain version and process exit
attestations rely on the primary agent's recorded execution results.
