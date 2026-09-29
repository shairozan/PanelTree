# Sprint 04 verification

## Red–green evidence

- SVG/text positive tests failed on `vector/text not implemented` and `measurement not implemented`; implemented bounded SVG parsing and shared real-font planning/rasterization; tests passed.
- App integration failed because text required a measurement provider; connected the default built-in provider and context-aware leaf renderer; inspect/build passed.
- Bundle relocation/manual-edit acceptance failed on `bundle not implemented`; implemented frozen dependencies, portable YAML, SVG/native text and provenance publication; test passed. CLI acceptance failed on `unknown flag: --bundle`; added the flag and it passed.
- Font-companion preservation failed on a missing relocated `.ttf.LICENSE`; preserved the companion path and reran successfully.
- Review regressions: a valid wide contained SVG failed raster limits; exported panels lacked stable authored identities after reordering; repeated-winding polygons rendered transparent. Fit-aware raster sizing, authored panel/layer identity keys and nonzero winding corrected these failures.
- Added explicit rejection of missing SVG namespace after a regression showed nonportable source was accepted. Supported fixtures all declare the standard SVG namespace.

## Acceptance coverage

The demo includes independent PNG environments/characters, SVG lantern/caption/FX, a group alpha mask and explicit-font lettering. Tests cover width-dependent real measurement, missing fonts, unsupported characters/SVG features, overflow, raster alpha, bundle relocation, preserved manual SVG edits and stable panel IDs. Relocation hides the original project and reproduces the exact PNG bytes from the portable page. Font license discovery survives relocation.

Inspected the 800×1200 PNG and loaded the portable SVG over a loopback-only static server in the browser. Caption text, lantern, FX, transforms, opacity and mask display correctly. Native SVG text remains text; sources are separate relative assets. Browser smoothing differs slightly at raster and mask edges, as documented. This does not claim validation in every external SVG editor.

## Final checks and review

Formatting, go test ./..., go test -race ./..., go vet ./... and golangci-lint run all passed after corrections; lint reported zero issues. Independent review via .agents/skills/paneltree-code-review/SKILL.md identified the SVG fit, polygon winding and stable panel-ID findings described above. All were fixed and independently re-reviewed with no remaining blocking code findings. Two documentation accuracy notes were corrected. Final PNG and SVG were visually checked again; the browser screenshot is in ignored bin/sprint-04-svg-preview.png. Changes remain local; hosted CI, push and issue closure are pending.

## Limitations

Basic Latin, bounded SVG shapes and explicit fonts only. No SVG-edit import, advanced shaping, PSD round-trip, PDF, persistent cache or asynchronous jobs. Details and limits: `docs/editable-bundles.md`.
