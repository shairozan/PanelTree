# SVG, lettering and portable bundles

## Usage

```sh
go run ./cmd/paneltree init my-book
# The destination root must exist; each invocation creates a distinct bundle.
go run ./cmd/paneltree build my-book/project.yaml --page page-01 --bundle my-book --width 800 --height 1200
# PNG-only export remains available, with the same supported sources.
go run ./cmd/paneltree build my-book/project.yaml --page page-01 --output my-book/preview.png
```

`--output` and `--bundle` are mutually exclusive. The build result supplies `bundle`, `build_id` and the preview `output` path. A bundle contains:

- `page.png`: the flattened preview.
- `page.svg`: separate nested groups for panels and layers, local transforms, clips, alpha masks, opacity and editable native text. The XML `id`, `data-kind`, `data-id` and `data-node` attributes retain authored panel/layer identities. Panel/layer IDs survive layout reordering; anonymous row/column containers use structural paths.
- `page.yaml`: a standalone portable page with relative dependency paths. It can be rebuilt after moving the whole bundle and without access to the original project.
- `composition.json`: schema/version, build ID, authoring page, resolved hierarchy, world/local geometry, source metadata and dependency provenance with SHA-256 hashes.
- `assets/`: exact original PNG/SVG/font/mask bytes, discovered font licenses, and derived white-alpha mask PNGs for SVG interoperability. The original masks remain intact.
- `originals/`: the original selected page YAML, retained byte-for-byte as provenance. Its original relative references are archival; rebuild with the portable `page.yaml`.

Build identity hashes the bundle version, portable page, resolved scene/output and dependency provenance. Identical inputs and output settings yield the same ID. A random publication suffix makes every build a new directory, including identical builds, so hand edits in prior bundles are never overwritten. Dependencies are frozen into private staging before measurement/rendering. Complete files are published by renaming staging into a newly reserved `<build-id>-<suffix>/bundle` directory on the same filesystem. Handled failures clean staging; a process crash may leave a `.paneltree-*` directory. Bundle publication does not implement a persistent cache.

Open the SVG in an SVG-capable editor to move or scale independent groups. Fonts are embedded in the SVG and retained as files. Text has explicit per-character x positions from the shared font metrics, so wrapping does not depend on an editor's automatic layout. Editing the string substantially may require adjusting those positions in the editor. Changes to exported SVG are **not imported** into YAML or composition metadata; edit `page.yaml` to change future automated builds. PSD round-trip, advanced shaping and PDF remain out of scope. Browser/editor smoothing can differ from the nearest-neighbor PNG preview, so pixel-identical rendering across SVG editors is not promised.

## Supported SVG source subset

The bounded SVG adapter supports a single root with `xmlns="http://www.w3.org/2000/svg"`, required positive unitless `width`/`height`, and optional `viewBox="0 0 width height"` matching those dimensions. Dimensions are at most 8192 per axis. Supported descendants:

| Element | Supported attributes |
| --- | --- |
| `g` | `id`, inherited `fill` |
| `rect` | `id`, `fill`, `opacity`, `x`, `y`, positive `width`, `height` |
| `circle` | `id`, `fill`, `opacity`, `cx`, `cy`, positive `r` |
| `ellipse` | `id`, `fill`, `opacity`, `cx`, `cy`, positive `rx`, `ry` |
| `polygon` | `id`, `fill`, `opacity`, 3–256 coordinate pairs in `points` |

Fills are `#RRGGBB`, `#RRGGBBAA`, or `none`, defaulting to black. Polygon filling uses SVG's default nonzero winding rule. Shape opacity is 0–1. Group opacity is deliberately unsupported in source SVG; use a PanelTree layer group for opacity. XML comments and the XML declaration are permitted. Unsupported elements/attributes, paths, CSS, strokes, intrinsic transforms, text, nested SVGs, external resources, scripts, filters, gradients, directives and foreign/missing namespaces fail explicitly. No URLs are fetched.

Each file is limited to 1 MiB, 256 elements, depth 32, and finite numeric magnitudes at most 1,000,000. Rasterization honors the assigned frame's contain/cover policy and transform-derived resolution; leaf rasters are limited to 8192 per axis and 4,194,304 pixels. Extremely elongated cover images can exceed this bound and fail. Curves/edges are sampled without antialiasing in the PNG adapter. The editable bundle retains the original SVG source, which can be edited separately.

## Lettering

```yaml
source:
  kind: text
  text: 'The moon is closer tonight.'
  font: ../assets/Go-Regular.ttf
  font_size: 30
  color: '#182539'
```

The font path is relative to the page. There is no system-font lookup or fallback. The default shared service uses the pinned `golang.org/x/image/font/opentype` adapter for TTF/OTF faces; the demo includes Go Regular and its license. Custom fonts can provide a companion `<font filename>.LICENSE`, which bundles preserve alongside the font.

Initial coverage is printable ASCII/basic Latin U+0020–U+007E plus newline, at most 4096 bytes. Tabs, controls, non-Latin characters and missing glyphs are rejected. Runs of spaces normalize to one space, with leading/trailing spaces removed within each paragraph; explicit newlines retain empty lines. Wrapping breaks at spaces, never splits a word, and fails on an overlong word or height overflow. Font size is in logical units, in (0,512]. Frames must exceed two logical units per axis and be at most 8192; one-unit padding is reserved around text. Font files are bounded to 8 MiB. Hinting is disabled and layout uses the same font advances, kerning, bounds and line metrics as raster/SVG placement. Resolution changes do not reflow text.

`inspect` now uses real text measurement by default. Applications can still inject a measurer or rasterizer for PNG-only workflows. Portable bundle builds currently require the built-in providers so frozen dependencies, native SVG text and provenance agree. Bounded decoding is not a hardened sandbox for adversarial font files; the upstream font parser has the same limitation.

Bundle dependency reads are capped at 128 MiB per build (repeated references count toward this budget), embedded unique font data at 16 MiB, and editable SVG at 64 MiB. Existing raster/composition limits still apply. See [rendering](rendering.md) for source preservation, PNG limits and masking conventions.
