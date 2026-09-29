# Incremental builds

Builds use `.paneltree/cache` beside the supplied project YAML. The directory is disposable and ignored by Git. `--cache-dir PATH` shares a cache across books or relocated projects; `--no-cache` bypasses cache reads and writes. Output paths must still be new, and bundles always receive a fresh directory. Cached work never replaces a source or a hand-edited export.

```sh
paneltree build my-book/project.yaml --page page-01 --output first.png
paneltree build my-book/project.yaml --page page-01 --output second.png
paneltree build my-book/project.yaml --page page-01 --output fresh.png --no-cache
```

The second build reports zero leaf renders and zero recompositions. It still reads and validates the project, snapshots source dependencies, measures text, verifies reused blobs, copies the finished output and, for bundles, writes fresh editable metadata/SVG. This is not a zero-I/O build.

## Dependency graph

`internal/build` plans separate production, composition and PNG export recipes. The canonical JSON recipe's SHA-256 is its identity. A different SHA-256 identifies the resulting bytes; recipes that produce identical bytes share a blob. Go JSON encoding orders map keys; parsed YAML comments and key order do not enter the graph.

Production recipes contain source/font byte hashes, renderer algorithm/version, source settings, explicit draft controls, and the context actually consumed by that adapter. PNG decoding ignores placement and output dimensions. SVG/text production includes frame and requested pixel dimensions; text includes its explicit font bytes. Translation is excluded from production. No machine paths, file timestamps or implicit random seeds enter production identity.

Composition recipes contain ordered input recipe keys, geometry, clipping, masks, opacity, fit, output dimensions/transform, and compositor version. Every layer, group, panel, layout container and page has a transparent composition node. Background and PNG encoder version belong to the final export recipe. Dependencies use recipe keys, so changing a draft invalidates ancestors even if its pixels happen to match an older draft.

| Change | Work invalidated |
| --- | --- |
| Move a character | Its placement and ancestors; existing artwork is reused |
| Edit dialogue or font bytes | Lettering production, placement and ancestors |
| Edit a shared source | Dependent branches on each page when that page is built |
| Change a mask | Mask production and masked branch/ancestors |
| Change background | Final PNG export; transparent compositions are reused |
| Comment, map order, timestamp, relocate/rename identical assets | None |
| Change renderer identity | Production using that identity and its ancestors |

Current rendering/encoding identities include the Go runtime version because standard-library behavior can change. Built-in identities also name the pinned font/vector dependency version. Upgrades may conservatively rebuild more work than a source edit.

Build JSON contains `cache` counters and graph `Events`. Each event exposes recipe, output content hash when executed, ordered input recipes, decision and reason. `built` with `missing-recipe` indicates required work, `hit` with `validated-hit` indicates verified reuse, and `skipped` with `ancestor reused` means the node was not executed or separately verified. Corruption reasons distinguish invalid records, blobs and raster payloads. These explain the current build; the MVP does not retain a chronological comparison with previous graphs.

## Explicit drafts

```yaml
source:
  kind: image
  path: ../assets/hero.png
  draft: {revision: 2, seed: 42}
```

Revision and seed are unsigned 64-bit values. Omitted values mean fixed zero. Increment the revision to request a new recipe; choose a different seed deliberately for another stochastic variant. Static adapters preserve these controls in identity but do not alter existing artwork. Future generators must consume the seed and identify all model/workflow dependencies. A seed is not a cross-GPU reproducibility guarantee. Generative execution and approval pins remain later-sprint work.

## Integrity and publication

Recipes and blobs are immutable. A recipe producing different bytes is an error requiring an explicit identity change. Writes go to temporary files, sync/close, and publish with an exclusive hard link: the blob first, recipe record last. A reader therefore never sees a partially written record. Filesystems without hard links return an error. Interrupted publication can leave an unreachable blob or temporary file; no automatic garbage collection is implemented.

Reads validate bounded regular files, record identity, byte count and SHA-256. Bad entries are quarantined and rebuilt; permission/I/O errors surface instead of silently disabling the cache. Raster payloads validate dimensions and limits before decoding. Premultiplied RGBA intermediates use an exact byte envelope because a PNG round-trip can alter low-alpha color values. Final exports remain PNG. Cached and uncached builds use the same isolated container grouping, which can differ by a rounding unit from Sprint 04's direct container painting.

Integrity validation is demand-driven: a page hit skips unused descendants, so corruption in an unused child is discovered only when that child is needed. Integrity hashes detect accidental corruption; a local cache is not a security boundary against an attacker who can replace both records and blobs.

## Adapter boundary and limits

`render.CacheRasterizer` adds a deterministic `CacheRecipe` contract to rasterization. The recipe must name every consumed dependency by bytes and relevant context, plus algorithm/version and seed. The caller must freeze those dependencies until rendering finishes. The default app service currently enables caching only for its built-in renderer and measurer, where the existing snapshot stage provides that guarantee. Injected adapters continue to render uncached. Future ComfyUI/Blender adapters must implement snapshot/version contracts before service opt-in.

Composition blobs currently store whole-output surfaces, including transparent pixels. This favors simple correct reuse over disk efficiency. The compositor retains its 256 MiB live-surface bound. Parents reuse a completed first child's surface; pending sibling accumulators count toward the bound. Per-blob limit is 128 MiB, and existing source/output limits still apply. No cache eviction, distributed execution, filesystem watcher, automatic multi-page build or lock/approval state is introduced. Per-page builds share recipes when using the same cache directory. Editable bundle metadata and SVG are regenerated even when PNG production is reused.
