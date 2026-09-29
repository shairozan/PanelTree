# PNG page rendering

Page export supports PNG, bounded SVG and explicit-font text without external services. See [editable bundles](editable-bundles.md) for Sprint 04 sources and portable exports.

```sh
go run ./cmd/paneltree init my-book
go run ./cmd/paneltree build my-book/project.yaml --page page-01 --output my-book/page-01.png
go run ./cmd/paneltree build my-book/pages/01.yaml --output my-book/vertical.png --width 1080 --height 1920 --fit contain
```

For a book or chapter with several pages, select a page ID. A standalone page needs no selector. Duplicate matching IDs require specifying the page YAML directly. Output parents must exist. Output files must be new: rebuilding requires a different destination or explicitly removing your own previous export. There is no overwrite flag. PNG encoding finishes in a temporary file beside the destination before an exclusive hard link publishes it. Existing files, including symlinks, source assets and manually edited exports, are protected. A filesystem without hard-link support fails explicitly. Handled errors clean temporary files; a process crash can leave a `.paneltree-*.png` temporary file.

## Composition

The shared `app.Service.Build` loads and validates the project, selects a page, resolves layout, renders it and exports the result. CLI factories use the same service; future MCP/web callers can do so too. `render.Rasterizer` is the injectable existing-pixels seam (`app.WithRasterizer`); the PNG adapter reads PNGs with `kind: image`. It does not apply placement. The separate `render.Renderer` artifact-generation contract remains available for future generation adapters. Neither interface puts a vendor workflow into story YAML.

Layers are painted in YAML order, first at the back. Local normalized frames, center-based scale/rotation and inherited transforms follow [layout conventions](layout.md). PNGs default to centered `contain`; `cover` crops to the layer frame. Sampling uses nearest neighbors and pixel centers, with numerical tolerance at shared boundaries. Rotation is supported but edges are not antialiased yet. Color blending uses Go's premultiplied 8-bit RGBA source-over semantics, without ICC management or linear-light conversion.

Groups compose into isolated transparent surfaces before their opacity is applied, so overlapping children do not accumulate group opacity multiple times. Groups do not implicitly crop their children. Panel rectangles and the output canvas clip all descendants. Each mask is a PNG relative to the declaring page; its **alpha channel** is stretched across the layer/group's local frame and transformed with it. Mask RGB is ignored: an opaque black-and-white PNG is fully opaque as a mask. A mask clips content outside its frame. Child masks apply before group masks and opacity.

Page backgrounds accept `#RRGGBB` or `#RRGGBBAA`; omission is transparent. The page background also fills letterboxing. Leaves, source PNGs and authoring YAML stay separate and unmodified. The exported page itself is flattened; use YAML and `inspect` for layer identities, bounds and subsequent movement. The optional `--bundle` export supplies editable SVG groups and portable source assets.

## Bounds and errors

- PNG files must be regular files, at most 32 MiB, 8192 pixels per axis and 4,194,304 pixels total. Header dimensions are checked before decompression.
- Output is limited to 8192 pixels per axis and 16,777,216 pixels total.
- Simultaneously live composition surfaces are limited to 256 MiB. Decoded inputs, masks, encoder state and Go runtime/GC overhead are additional; this is not an operating-system memory cap. Deep groups at large resolutions can exceed the surface budget and fail rather than allocate indefinitely.
- Composition accepts at most 4096 visited nodes and depth 128. Context cancellation is checked during decoding, scanlines, encoding and before publishing.

Errors identify a missing/corrupt leaf or mask and include page/layer context. Unsupported SVG features, missing fonts and lettering overflow fail explicitly; see the bounded adapters in Sprint 04. No persistent cache, job queue, automatic generation, PDF or motion output is included yet.

## Reproducible example

`tools/demo-assets` creates the original CC0 PNG fixtures under `internal/project/template/assets` when run from the repository root. The SVG fixtures are now rendered by the bounded SVG adapter. The five-panel page exercises a shared environment and character, close-up crop, rotation/scale, group opacity and an alpha mask. These are deliberately simple diagnostic illustrations. The generator replaces its three named PNG development fixtures and the bundled Go Regular font; normal `init` and `build` preserve existing user work.
