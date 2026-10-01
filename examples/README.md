# Two-page demonstration book

Generate the licensed canonical example with `paneltree init NEW_DIRECTORY`.
The [embedded template](../internal/project/template) is its single source of truth;
the ordinary demo is generated from that template.

Page 1 demonstrates weighted/percentage tracks, PNG characters/backgrounds,
SVG props/FX, explicit-font lettering, rotation, opacity and a group alpha mask.
Page 2 isolates the shared character against transparency. Both build without AI.
Follow the [walkthrough](../docs/getting-started.md) or run `go test ./acceptance -v`.

Original geometry is CC0, Go Regular has its own BSD-style license, and application
code is MIT. See [licensing](../docs/licensing.md).

The separate [character evaluation fixture](characters/project.yaml) shares one
[package](characters/alex.json) across three views. Its reference assets are
copies of the canonical CC0 geometric hero and lantern, with the original notice.
They are fixed evaluation inputs. See [character usage and rubric](../docs/characters.md)
and [recorded GPU results](../docs/sprints/11-verification.md).
