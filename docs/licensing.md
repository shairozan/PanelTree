# Licensing and attribution

The project owner selected **MIT** on 2026-09-30 during Sprint 09, replacing the earlier Apache-2.0 proposal. [LICENSE](../LICENSE) applies to PanelTree's original application code/documentation. It does not relicense dependencies, fonts, imported artwork or user projects.

## Dependency review

Pinned executable dependencies were inspected from downloaded Go modules, including root license/NOTICE files and nested license locations. Windows/Linux/macOS dependency lists were compared. Linked modules use MIT, BSD-style and Apache-2.0 terms; no reciprocal source-disclosure requirement was found in this inventory. Separately licensed application code can use these dependencies while preserving their terms and notices. Apache dependencies retain Apache conditions; the distribution is not exclusively MIT.

[THIRD_PARTY_NOTICES.txt](../THIRD_PARTY_NOTICES.txt) preserves versioned upstream license text, YAML's NOTICE, Go runtime licensing, and fixture/font notices. MCP Go SDK v1.8.0 describes an Apache/MIT transition and CC-BY-4.0 documentation; its complete statement is retained instead of treating it as MIT-only. PanelTree does not redistribute SDK documentation. Go YAML v3 has per-file MIT/Apache terms; both are retained. The nested Segment encoding fuzz-data license applies to upstream fuzz fixtures, which are not included in the executable or this repository.

Every release artifact includes the root LICENSE and third-party notices alongside its binary. Keep those files when redistributing. Source dependencies remain separately fetched Go modules with upstream notices; no vendored files were modified. Future copying, vendoring or modification must retain notices and identify modifications as required.

Review sources: [MIT text](https://opensource.org/license/mit), [Apache definitions and redistribution conditions](https://www.apache.org/licenses/LICENSE-2.0), and the exact upstream terms preserved in the notices. This records the reviewed versions, not a guarantee for future dependencies or every possible distribution arrangement.

## Demo book

The two-page book is embedded from `internal/project/template` and created by `paneltree init`. Original geometric PNG/SVG fixtures are CC0 1.0; attribution is in `assets/LICENSE.txt`. Go Regular was created by Bigelow & Holmes for the Go project and keeps its BSD-style license in `assets/Go-Regular.ttf.LICENSE`. The font is not CC0 or MIT. Initialized projects and portable exports preserve the font companion license. These are diagnostic assets, not AI-generated art.

Imported fonts, characters and artwork retain their applicable rights/licenses. PanelTree does not grant rights to those assets or require outputs to adopt the application's license.

## Maintenance

After dependency/toolchain changes, run `go list -deps -f '{{if .Module}}{{.Module.Path}} {{.Module.Version}} {{.Module.Dir}}{{end}}' ./cmd/paneltree` for Windows/Linux/macOS with CGO disabled, inspect root/package-specific licenses and NOTICE files, and refresh the consolidated file. Compare linked modules, not every `go.sum` entry. Include changed Go runtime/font notices and inspect new embedded assets separately. The root MIT license never replaces dependency obligations.
