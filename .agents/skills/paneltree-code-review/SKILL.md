---
name: paneltree-code-review
description: Independently review PanelTree implementation, tests, and CI changes against the active sprint and repository engineering requirements before completion.
---

# PanelTree code review

Run this skill in an independent review agent. Read repository AGENTS.md, the active ticket or supplied acceptance criteria, and the complete tracked and untracked change set. Review only; do not edit, commit, close issues or publish comments.

Check observable correctness, failure paths, source preservation, configuration isolation, schema validation, and compatibility with the shared service/renderer boundaries. Check that tests exercise requirements and that claimed red–green evidence describes actual behavior failures. For workflow changes, check event coverage, permissions, dependency gates, artifact names and target matrix, and distinguish local validation from a hosted run.

Trace likely bugs to a concrete input or scenario. Run focused read-only checks when useful. Report actionable findings with severity, file/line, reproduction and impact. Avoid speculative preferences and stylistic churn. If no actionable findings remain, say so and identify unverified areas. The implementing agent must fix findings and request re-review when corrections are substantive.
