# Sprint 11 character fidelity RCA

The evidence points to a combination of text-only conditioning, prompt
construction and model limitations. We have not isolated a checkpoint-specific
cause by comparing models. Different views need not be identical; the relevant
failures are loss of intended identity cues, costume, prop and requested framing.

## Verified inputs

The original three frozen recipes use the same SD 1.5 checkpoint, seed 17,
512-square resolution, Euler sampler, 20 steps, CFG 7 and negative prompt.
Only the selected pose phrase changes. The package's reference artwork is
recorded and hashed, but no image-conditioning nodes or reference pixels are
passed to ComfyUI. The text prompt contains identity, hex palette values,
costume, expression, pose and prop descriptions.

The model therefore receives separate text-to-image requests with no persistent
character identity representation. Sharing the package and seed does not make
this a reference-conditioned identity workflow. Image prompt adapters are a
separate mechanism; see the [IP-Adapter implementation](https://github.com/tencent-ailab/IP-Adapter).

## Controlled local experiments

Fifteen requests were evaluated on the same installed model/runtime. Five prompt
variants were run at each of seeds 17, 18 and 19; all other workflow settings
were held constant, apart from the output filename prefix. Requests were sent
directly to ComfyUI for diagnosis; no production mapping was changed.

| Variant | Tokens excluding special tokens | Change |
| --- | --- | --- |
| Original front | 99 | Existing package-expanded prompt |
| No hex palette | 76 | Remove only the palette clause |
| Plain palette | 87 | Replace only the palette clause with ordinary color names |
| Concise front | 48 | Shorter description, explicit head-to-boots framing and colorful flat style |
| Concise side | 49 | Same concise prompt, requesting strict left-facing side profile |

Counts were measured with the installed CLIP tokenizer. ComfyUI's actual
`SD1Tokenizer` confirmed two chunks for the original and one for each concise
prompt. The original palette clause consumes 23 tokens. The original pose and
lantern instructions fall late in the prompt. ComfyUI chunks this input rather
than simply truncating it: claiming that those instructions were discarded
would be incorrect.

Observed results:

- Repeating the original seed-17 input visually reproduced the sepia bust.
  Original seeds 18 and 19 produced different crops/styles; seed 18 included a
  lantern but cropped away the head. The initial seed was not representative of
  every output the model can produce.
- Removing only the hex clause changed style and color, but did not reliably
  produce the orange coat, lantern or full-body framing. Plain color words also
  did not reliably bind the requested colors to the intended garments.
- Concise prompts produced more colorful illustrations and recognizable
  lanterns in seeds 18 and 19. Seed 17 still produced an ambiguous handheld object
  in the front view. This rewrite changes length, wording, ordering and style
  instructions together; it does not isolate chunk count as the causal factor.
- None of the fifteen images clearly satisfied head-to-boots full-body framing.
  None of the three concise side prompts yielded a strict side profile. Those
  failures persist after prompt simplification.
- Faces, hair, accessories and costume details still varied across prompt/seed
  changes. The experiments did not establish stable identity or exact prop fidelity.

## Diagnosis and confidence

**Confirmed workflow limitation:** no reference-image conditioning. The existing
package improves reuse and traceability, but currently cannot anchor identity to
the reference artwork.

**Demonstrated contributor:** our prompt representation materially changes the
outputs. Hex strings consume substantial token space, and the package expansion
creates a long list of competing details. The controlled palette comparisons
show sensitivity, while the concise rewrite improves some visible properties.
Neither proves that hex strings alone caused the original failures.

**Likely contributor:** SD 1.x's limited composition/attribute binding and human
depiction capabilities. The original [Stable Diffusion v1 model card](https://github.com/CompVis/stable-diffusion/blob/main/Stable_Diffusion_v1_Model_Card.md#limitations)
documents those limitations. Continued framing and pose failures are consistent
with them, but a model comparison is needed to measure the checkpoint's share
of the problem.

**Not demonstrated:** a PanelTree transport/compositor defect, VRAM shortage,
wrong checkpoint, or missing VAE. Frozen graphs carry the expected inputs and
the failures are already visible in raw ComfyUI outputs before composition.
The same graph/model can produce colorful output with a prompt change.

## Next experiments

1. Keep palette hex codes as structured package data, but test a concise,
   prioritized renderer prompt using natural-language garment colors. Evaluate
   the same seed set before adopting any production mapping change.
2. Compare the same evaluation suite against another checkpoint with appropriate
   model-specific settings. Report that resolution/workflow differences may
   confound a cross-family comparison; do not assume a model swap fixes identity.
3. If stronger identity consistency becomes necessary, separately evaluate image
   conditioning with suitable reference art. Test pose control separately from
   identity. The current geometric demo is a diagnostic reference, not a rich
   multi-view character sheet.

All fifteen PNGs, exact prompts, workflow graphs and prompt IDs remain local in
ignored `bin/sprint11/rca/`. `results.json` contains the original/concise trials;
`palette-results.json` contains the isolated palette trials. This is a small,
manual diagnostic sample, not a benchmark or statistical attribution. No new
model was downloaded and no production behavior was changed.
