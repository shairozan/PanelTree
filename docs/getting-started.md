# Fresh-checkout walkthrough

Install Git and Go 1.27.1 or newer. A first build needs access to Go's module download service; `go.mod`/`go.sum` pin dependencies. Runtime use needs no Python, Node, GPU, ComfyUI, API credentials or AI service. Use ordinary local directories, not symlink/junction paths for MCP.

```sh
git clone https://github.com/shairozan/PanelTree.git
cd PanelTree
go mod download
go test ./...
```

Linux/macOS:

```sh
go build -trimpath -o bin/paneltree ./cmd/paneltree
./bin/paneltree init bin/my-book
./bin/paneltree validate bin/my-book/project.yaml
./bin/paneltree build bin/my-book/project.yaml --page page-01 --bundle bin/my-book --width 800 --height 1200
./bin/paneltree build bin/my-book/project.yaml --page page-02 --bundle bin/my-book --width 800 --height 1200
./bin/paneltree build bin/my-book/project.yaml --page page-01 --output bin/my-book/warm.png --width 800 --height 1200
```

Windows PowerShell:

```powershell
go build -trimpath -o bin/paneltree.exe ./cmd/paneltree
.\bin\paneltree.exe init bin/my-book
.\bin\paneltree.exe validate bin/my-book/project.yaml
.\bin\paneltree.exe build bin/my-book/project.yaml --page page-01 --bundle bin/my-book --width 800 --height 1200
.\bin\paneltree.exe build bin/my-book/project.yaml --page page-02 --bundle bin/my-book --width 800 --height 1200
.\bin\paneltree.exe build bin/my-book/project.yaml --page page-01 --output bin/my-book/warm.png --width 800 --height 1200
```

`init` requires a new directory; use another name on repeat runs. Build JSON gives the exact `bundle` and `output` paths. Open `page.png` for the flattened page and `page.svg` in an SVG-capable browser/editor for independent groups and lettering. Keep the bundle's `assets/`, `page.yaml` and `composition.json` beside it. SVG-editor fidelity varies; see [editable bundles](editable-bundles.md).

The third build reuses page 1's cache and reports zero leaf renders/recompositions. Existing output files are protected, so choose a new filename each time. Preserve the whole book directory, including `.paneltree/`: it contains editorial state, durable pins and jobs as well as disposable cache. It is ignored by Git, so Git alone does not back up those selections. See [editing](editing.md).

## Verify MCP without an agent

```sh
go test ./acceptance -run TestMVPExecutableWorkflow -count=1 -v
```

This compiles a new executable, initializes the two-page demo, builds/relocates both exports, compares warm images, starts the real stdio server, and drives it using the Go MCP SDK. It requests/runs/selects a static asset, tests revisions/locks across CLI/MCP, retrieves PNG/SVG resources and verifies durable cancellation. Files live under the test's temporary root and are cleaned up. No model is involved.

For your own MCP client, create a dedicated existing root such as `C:/Books` or `/home/artist/books`. Configure the absolute executable path with arguments `mcp`, `serve`, `--root`, and that root. Start it through the client; typing into its terminal is not an MCP session. See [client configuration and tools](mcp.md). An agent can initialize/open projects, request assets, run jobs and build pages under the same revision/lock policy as the CLI.

## Reference

- [Schema](schema.md), [layout](layout.md), [rendering](rendering.md), [editable exports](editable-bundles.md).
- [Runtime configuration](../README.md#runtime-configuration), [MCP roots](mcp.md#configure-a-client).
- [Artwork protection](editing.md), [jobs and recovery](jobs.md), [caching](incremental-builds.md).
- [Architecture/adapters](architecture.md), [acceptance](acceptance.md), [contributing](../CONTRIBUTING.md), [licensing](licensing.md).
