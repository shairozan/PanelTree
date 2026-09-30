# Two-page demonstration book

Generate the licensed canonical example with `paneltree init NEW_DIRECTORY`.
The [embedded template](../internal/project/template) is its single source of truth;
this directory does not duplicate assets that could drift.

Page 1 demonstrates weighted/percentage tracks, PNG characters/backgrounds,
SVG props/FX, explicit-font lettering, rotation, opacity and a group alpha mask.
Page 2 isolates the shared character against transparency. Both build without AI.
Follow the [walkthrough](../docs/getting-started.md) or run `go test ./acceptance -v`.

Original geometry is CC0, Go Regular has its own BSD-style license, and application
code is MIT. See [licensing](../docs/licensing.md).
