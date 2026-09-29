# Project schema v0.1

Every UTF-8 YAML file has `schema: paneltree/v0.1` and exactly one `book`, `chapter`, or `page` mapping. A book's `chapters` and a chapter's `pages` are ordered lists of relative file paths, resolved against the declaring file. Documents cannot be shared twice in a book; asset files can. A page or chapter can also be loaded directly. `title` is optional on books and chapters.

IDs are explicit strings matching `[A-Za-z0-9][A-Za-z0-9_-]*`. Document IDs are unique across the loaded project. Panel IDs are unique within a page; layer IDs are unique across an entire panel, including nested groups. Quote numeric IDs. Ordered lists preserve chapter/page order, layout order and back-to-front layer order.

Pages require `id`, positive finite `canvas.width` and `canvas.height`, a `layout`, and `panels`. Background is an optional color string. A layout is either a `panel` reference or a `type: row|column` with nonempty `children`. Optional `size` selects one positive `weight` or `percent` in `(0,100]`. `margin` and `gutter` are nonnegative logical design units on containers. Every panel must be placed exactly once. Structural validation checks schema; inspection also resolves geometry and rejects aggregate overconstraints. See [layout conventions](layout.md).

Panels contain `id` and ordered `layers`. Empty panels are valid. A layer has an `id`, optional semantic `role`, and exactly one `source` or nonempty `children` list. A character can therefore be a leaf or a group. Roles are descriptive, not backend names.

Supported source definitions:

```yaml
source: {kind: image, path: ../assets/hero.png}
source: {kind: svg, path: ../assets/prop.svg}
source: {kind: text, text: "Hello", font: ../assets/dialogue.ttf, font_size: 30, color: '#111111'}
```

Image/SVG sources require a path and reject text-only fields. Text requires text, font and positive font_size, without a path. This sprint does not open or rasterize asset files; missing asset checks arrive with renderers. Paths are interpreted relative to the page document; inspection reports source file locations. Imported references can traverse `..`; this local loader is not a sandbox. The later workspace/MCP boundary must enforce configured roots.

Layers may specify `frame: {x, y, width, height}` in parent-normalized coordinates, `fit: contain|cover`, `transform: {scale_x, scale_y, rotation}`, `opacity`, and `mask`. Frame dimensions and scales must be positive finite values; positions/rotation must be finite and opacity is `[0,1]`. Omitted frames/scales remain absent for the resolver to default to full-parent/unit-scale. Negative/outside-parent positions are permitted; clipping is a later compositor responsibility. A mask is an asset path. Semantic color/font interpretation belongs to the relevant renderer.

Unknown fields, duplicate keys, explicit nulls, aliases/anchors, invalid unions, unsupported schema/source kinds, duplicate IDs, missing document/panel references and cycles fail validation. Errors include file, line and column where available; YAML syntax/type messages retain the parser's detailed location. Limits: 1 MiB per YAML document, nesting/reference depth 64 and 1,024 loaded documents. These bound malformed projects; they are not a complete hostile-input sandbox.

`inspect` emits the typed document tree, ordered pages and a semantic revision hash. Revision hashes include parsed authoring documents and declared paths, not file timestamps, comments or absolute machine paths. They do not yet fingerprint asset bytes; Sprint 05 introduces rendering content keys, and Sprint 06 adds edit concurrency semantics.

See `internal/project/template` for the canonical two-page example. `paneltree init <new-directory>` copies it, including the five-panel 60/40-and-thirds page and two original CC0 SVG assets. The target's parent must exist; the destination itself must not. An interrupted initialization is retained for inspection and an ordinary repeat refuses to overwrite it.
