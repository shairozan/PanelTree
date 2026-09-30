# Architecture and adapter contracts

The authoring tree is Book → Chapter → Page → Panel → Layer/Component. YAML owns content and intent; resolved geometry, editorial state, cached work and exports are separate representations.

| Package | Responsibility |
| --- | --- |
| `model`, `internal/project` | Backend-neutral schema, IDs, strict loading and embedded demo |
| `scene`, `internal/layout` | Measure with assigned constraints, arrange recursively, return immutable scenes |
| `render`, `internal/adapters` | Leaf contracts, bounded PNG/SVG/text rendering and optional ComfyUI drafts |
| `internal/compose` | Transforms, clipping, masks, isolated groups and source-over composition |
| `internal/build`, `internal/cache` | Production/composition/export recipes and immutable blobs |
| `internal/workspace`, `internal/asset` | Revision transactions, recovery, ownership and protected artwork |
| `internal/jobs` | Frozen inputs, bounded workers, durable status and owner-death recovery |
| `internal/export` | Protected PNG publication and portable editable SVG/source bundles |
| `app` | Shared project, edit, job and build policy |
| `internal/cli`, `internal/mcp` | Cobra/Viper factories and root-constrained local stdio MCP |

Parents assign frames/context; leaves render for the final composition. ComfyUI does not own page planning, state or scheduling. It is an optional RGB draft renderer, not an MVP prerequisite.

## Adapter boundary

`render.Rasterizer` returns read-only existing pixels. `SceneRasterizer` also receives final logical bounds, requested pixel size, fit, source recipe and revision. Placement, clipping, masks and group opacity belong to the compositor. Adapters must honor cancellation, bound inputs/output allocations, and never mutate authoring files or select candidates automatically.

`CacheRasterizer.CacheRecipe` identifies every consumed byte dependency, algorithm/model/workflow version, explicit seed/revision and relevant context. Exclude machine paths and timestamps. Freeze dependencies until rendering completes; fail explicitly when unavailable. See [incremental builds](incremental-builds.md). The default service currently caches only its built-in renderer/measurer. Injection through `app.WithRasterizer`/`WithMeasurer` does not register a durable-job backend or editable SVG implementation.

`render.Renderer` describes a future file-artifact contract (descriptor, dependencies, request/result); it is not a plugin loader. ComfyUI implements the scene raster/cache-recipe contracts and integrates with durable jobs through frozen profiles, capability discovery, executor dispatch and candidate validation. Endpoint/profile settings belong in runtime configuration, outside story YAML. Selected generated PNGs use the ordinary build/cache/export path. See [ComfyUI generation](comfyui.md) for provenance and transport limits. Generic third-party registration is not implemented.

Public types are pre-1.0 contracts that can evolve with documented migrations and runnable fixtures; no stable third-party ABI is promised.

## Development and CI

Follow [AGENTS.md](../AGENTS.md) and [CONTRIBUTING.md](../CONTRIBUTING.md). Every push/PR runs tests, vet and native executable builds on Windows/macOS/Linux, Linux race checks, bounded YAML/layout fuzzing, lint and formatting. The executable acceptance test drives the compiled CLI and real stdio MCP server.

Semantic-version tags run the full CI gate before producing six CGO-disabled binaries (amd64/arm64 for Windows/macOS/Linux), SHA-256 checksums, LICENSE and third-party notices. Actions artifacts are downloadable; workflows do not publish GitHub Releases or create/push tags. Hosted results and branch protection are separate from local validation. Release publishing needs explicit authorization.
