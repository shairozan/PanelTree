# Architecture and adapter contracts

The authoring tree is Book → Chapter → Page → Panel → Layer/Component. YAML owns content and intent; resolved geometry, editorial state, cached work and exports are separate representations.

| Package | Responsibility |
| --- | --- |
| `model`, `internal/project` | Backend-neutral schema, IDs, strict loading and embedded demo |
| `scene`, `internal/layout` | Measure with assigned constraints, arrange recursively, return immutable scenes |
| `render`, `internal/adapters` | Leaf contracts and bounded PNG/SVG/text rendering |
| `internal/compose` | Transforms, clipping, masks, isolated groups and source-over composition |
| `internal/build`, `internal/cache` | Production/composition/export recipes and immutable blobs |
| `internal/workspace`, `internal/asset` | Revision transactions, recovery, ownership and protected artwork |
| `internal/jobs` | Frozen inputs, bounded workers, durable status and owner-death recovery |
| `internal/export` | Protected PNG publication and portable editable SVG/source bundles |
| `app` | Shared project, edit, job and build policy |
| `internal/cli`, `internal/mcp` | Cobra/Viper factories and root-constrained local stdio MCP |

Parents assign frames/context; leaves render for the final composition. ComfyUI does not own page planning, state or scheduling. It is an unavailable future capability, not an MVP prerequisite.

## Adapter boundary

`render.Rasterizer` returns read-only existing pixels. `SceneRasterizer` also receives final logical bounds, requested pixel size, fit, source recipe and revision. Placement, clipping, masks and group opacity belong to the compositor. Adapters must honor cancellation, bound inputs/output allocations, and never mutate authoring files or select candidates automatically.

`CacheRasterizer.CacheRecipe` identifies every consumed byte dependency, algorithm/model/workflow version, explicit seed/revision and relevant context. Exclude machine paths and timestamps. Freeze dependencies until rendering completes; fail explicitly when unavailable. See [incremental builds](incremental-builds.md). The default service currently caches only its built-in renderer/measurer. Injection through `app.WithRasterizer`/`WithMeasurer` does not register a durable-job backend or editable SVG implementation.

`render.Renderer` describes future artifact generation (descriptor, dependencies, request/result). It is not a functioning plugin loader. Integration requires capability discovery, frozen job inputs, executor dispatch and candidate validation, plus tests for stale results and protected selections. Endpoint/credentials belong in runtime configuration, outside story YAML. No generic registration or ComfyUI network execution exists yet.

Public types are pre-1.0 contracts that can evolve with documented migrations and runnable fixtures; no stable third-party ABI is promised.

## Development and CI

Follow [AGENTS.md](../AGENTS.md) and [CONTRIBUTING.md](../CONTRIBUTING.md). Every push/PR runs tests, vet and native executable builds on Windows/macOS/Linux, Linux race checks, bounded YAML/layout fuzzing, lint and formatting. The executable acceptance test drives the compiled CLI and real stdio MCP server.

Semantic-version tags run the full CI gate before producing six CGO-disabled binaries (amd64/arm64 for Windows/macOS/Linux), SHA-256 checksums, LICENSE and third-party notices. Actions artifacts are downloadable; workflows do not publish GitHub Releases or create/push tags. Hosted results and branch protection are separate from local validation. Release publishing needs explicit authorization.
