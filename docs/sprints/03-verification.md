# Sprint 03 verification

## Red–green history

- PNG read/alpha tests failed with `PNG not implemented`; composition fixtures for source-over, isolated group opacity, nested placement, rotation/scale, masks and contain/cover failed with `composition not implemented`. Implemented the static adapter and compositor; those tests passed.
- Shared-service demo export failed with `build not implemented`; CLI integration failed with `unknown flag: --page`. Implemented page selection, build orchestration, exclusive PNG publication, flags and PNG demo assets; tests passed.
- A shared half-pixel boundary regression reproduced a transparent seam at pixel (30,20) between panels. Matching inverse-sample tolerance to rounded edge conventions fixed it; the regression now passes.
- Additional failure-path coverage verifies oversized inputs/output, cancellation, missing/corrupt PNG errors, protected existing destinations, temporary-file cleanup, and unknown/ambiguous page selection.

## Visual verification

Rendered the five-panel demo at 800×1200 to the ignored local `bin/sprint-03-demo.png` and inspected it. Verified white margins/gutters, 60/40 upper panels and equal lower thirds, a separate transparent character, cropped close-up, rotated/scaled character, translucent character group and the oval mask over the final group. No visible leaks across panel boundaries. This is a geometric diagnostic example, not generated character art.

## Completion checks

Formatting, full tests, race tests, go vet and golangci-lint all passed after corrections; lint reported zero issues. Independent review through .agents/skills/paneltree-code-review/SKILL.md found fractional lower-edge overpainting. A new regression failed with an extra opaque column; rounded frame/content footprints and group-mask clipping corrected it. Independent re-review confirmed the fix with no remaining actionable findings. The final render was inspected again as bin/sprint-03-final.png. Hosted CI remains pending publication. No commit, push, release or issue closure is implied by local checks.

## Limits

Nearest-neighbor sampling; no antialiasing or color-profile management. Mask alpha is used, not luminance. Export is a flattened PNG; source assets and layer definitions remain independent. SVG/text, persistent caching, asynchronous jobs and layered export formats remain future work. See `docs/rendering.md` for allocation limits and publication behavior.

