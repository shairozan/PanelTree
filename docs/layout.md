# Measure and layout

Sprint 02 resolves geometry without producing pixels or generating assets. `app.Service.Inspect` returns the original authoring snapshot plus ordered `scenes`, each containing an immutable resolved tree and output transform. `validate` remains structural YAML validation; use `inspect` to check geometric feasibility. The semantic source revision remains independent of output sizing.

```sh
go run ./cmd/paneltree inspect internal/project/template/project.yaml
go run ./cmd/paneltree inspect internal/project/template/project.yaml --width 1080 --height 1920 --fit contain
```

## Allocation

A row divides width and a column divides height. Subtract twice the container margin on both axes, then subtract all main-axis gutters before allocating tracks. Percentages reserve that available extent first. Positive weights share the remaining extent; an omitted size means weight 1. Percentage-only children must total 100%. Reject excessive percentages, a leftover percentage-only remainder, exhausted margins/gutters, and zero space for weighted children. Cross-axis bounds fill the inner container.

Allocation remains floating-point in logical design units. The measure pass recursively proposes track bounds and requests leaf metrics at their actual assigned widths, rather than guessing text dimensions from the whole page. A second pass resolves local/world transforms, panel clips and output bounds. Fixed authored tracks are not expanded automatically to accommodate overflow. A provider's minimum size exceeding allocation is an error; for text, wrapped preferred dimensions must also fit. Groups preserve independent children.

The canonical 1200×1800 page has 40-unit margins, 24-unit gaps and two 848-unit rows. Its first two panels are 657.6 and 438.4 units wide; each lower panel is 1072/3 units wide. Tests assert these values and nested placement.

## Coordinates and transforms

`Bounds` is a node's untransformed rectangle in its parent's local space. Layers convert normalized frame coordinates into that rectangle; omission means the whole parent. `World` maps the node's own local coordinates (starting at 0,0) into page logical coordinates. `LogicalBounds` is its transformed axis-aligned bounding box in page coordinates.

Matrices use `[a,b,c,d,e,f]`, with `x'=a*x+c*y+e` and `y'=b*x+d*y+f`. Apply scale then rotation around the frame center, then placement into the parent, then ancestor transforms. Rotation is in degrees; positive rotation is clockwise on a canvas whose y-axis points down. Children inherit their parent's transform. The initial anchor is the center; author-selectable anchors and flipped scales are not part of this schema yet.

`Clip` is the enclosing panel rectangle in logical page space. Layer groups do not create extra implicit clips, so an oversized child can extend through a group until the panel clips it. Masks remain layer-local metadata applied before placement by a future compositor. `Opacity` is local to the node; a compositor must honor group opacity rather than baking it independently into every overlapping child. `Fit`, source, role and mask metadata remain available to leaf renderers/compositors.

## Output coordinates

Width and height must be supplied together or both omitted. Native output retains the identity transform and rounds canvas edges to pixel dimensions, including fractional logical dimensions. Explicit output aspect mismatch defaults to an error. `contain` centers the whole page with letterboxing; `cover` centers and crops it. Neither changes logical layout or reading order. The output object carries its affine `World` transform and the output-canvas pixel clip.

`Pixels` is a half-open, transformed bounding rectangle computed by rounding absolute left/top/right/bottom edges, not separately rounded widths. Values within a relative tolerance of 1e-12 (with a one-pixel floor) of a half-pixel threshold are snapped before rounding, preventing numerical noise from splitting shared edges. It is not pre-clipped: a cover export or rotated layer can extend outside the output. A renderer must intersect the transformed panel clip and output clip. Zero-width pixel footprints at tiny resolutions are permitted even when logical geometry is positive. Output dimensions are limited to 10,000,000 per axis and transformed pixel edges to ±1,000,000,000 to keep integer conversions bounded; these are geometry limits, not permission to allocate enormous raster buffers.

## Measurement seam

`scene.Measurer.Measure(context.Context, scene.MeasureRequest)` receives a value copy of the source, its declaring page directory and logical constraints. Providers return minimum and preferred sizes. Use `app.WithMeasurer(provider)` to inject one for inspection; provider errors retain their cause and include layer context. Providers must only use existing asset metadata/font metrics and must not generate assets or make network requests.

The CLI now supplies the explicit-font built-in text measurer by default; direct layout calls without a measurer still fail for text rather than inventing metrics. Image/SVG slots can resolve without intrinsic metrics. Direct calls without a provider mark them `Measured: false`; the default provider returns zero metrics for these fixed slots, so `Measured: true` records a provider call rather than intrinsic-size discovery. The built-in raster adapters interpret `Fit` during rendering. Unit tests inject a deterministic wrapping fake and verify measurement changes as assigned width changes.

`scene.Resolved` stores private data. `Tree()` deep-copies the tree, child slices and source pointers, so callers cannot mutate the stored scene or authoring model through it. JSON serialization exposes values only. The existing component interface remains an extension contract; this sprint's declarative resolver operates on the typed model directly.
