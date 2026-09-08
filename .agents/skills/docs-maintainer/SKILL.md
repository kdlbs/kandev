---
name: docs-maintainer
description: Keep public Kandev docs current when code or behavior changes affect CLI commands, config keys, install/deploy flows, workflows, executors, APIs, screenshots, or user-facing terminology. Use this before finishing any change with public documentation impact, and when reviewing whether a change needs docs.
---

# Docs Maintainer

Use this skill to decide whether public docs need updates and to make those updates in the right place.

## Docs Boundaries

- Public website docs source lives under `docs/public/**`.
- Product context, requirements, and system designs stay under `docs/specs/**`.
- Implementation plans and work orders stay under `docs/plans/**`.
- Architecture decisions stay under `docs/decisions/**`.
- Raw supporting notes can remain under `docs/**` outside `docs/public/**`, but do not publish them unless rewritten for users.
- `docs/public/meta.json` owns published-page order and navigation groups. Page paths own routes, and page frontmatter owns titles and descriptions.
- The landing/docs website generates its content from this directory. Do not hand-edit generated files in the landing repository.

## When Docs Need Updates

Check public docs when a change affects:

- CLI commands, flags, install commands, or runtime launch behavior.
- Configuration keys, environment variables, defaults, profiles, or feature flags.
- Workspaces, workflows, tasks, agents, executors, worktrees, Git behavior, or review flows.
- Docker, Kubernetes, service, desktop, remote environment, or Windows instructions.
- Public APIs, WebSocket messages, workflow import/export schemas, or integration contracts.
- Screenshots, visible UI labels, navigation, onboarding, or user-facing terminology.

Skip public docs when the change is:

- Purely internal refactoring with no behavior change.
- Test-only, fixture-only, or build-only without user-visible behavior.
- A speculative plan or design note that belongs in `docs/specs/**`, `docs/plans/**`, or `docs/decisions/**`.

## Workflow

1. Identify docs impact from the diff and changed behavior.
2. Search `docs/public/**` first for affected terms and commands.
3. If public docs exist, update them with the same PR as the behavior change.
4. If no public docs exist but the behavior is user-facing, add or propose the smallest useful public page/section.
   When adding a page, include `title` and `description` frontmatter and list its page slug or path without the `.md` extension in `docs/public/meta.json` exactly once, for example `cli`. See `docs/public/README.md`.
5. If the change only updates implementation intent or architectural history, update specs/plans/ADRs instead.
6. Classify each public page by its primary Diátaxis content type:
   - **Tutorial:** teach a beginner by leading them through one successful outcome.
   - **How-to guide:** help a reader complete a known task, with focused steps, choices, and recovery paths.
   - **Reference:** provide accurate, complete lookup information such as fields, commands, defaults, limits, or protocol contracts.
   - **Explanation:** build understanding of a concept, boundary, rationale, or trade-off.
   Keep one dominant type per page. Link to another page when a long section changes from learning to procedure, lookup, or explanation; do not force every page into a generic tutorial-shaped opening.
7. Keep public docs task-oriented and scan-friendly:
   - Tutorials should lead with prerequisites and a linear first success; how-to guides should lead with the task, expected result, and only the prerequisites it needs.
   - Reference pages should lead with scope and the contract readers need to look up; explanation pages should lead with the question or concept and why it matters.
   - Use short paragraphs (one idea, normally three sentences or fewer) and bullets for choices, limits, and consequences.
   - Prefer a link to the page that owns a detailed contract over repeating it.
   - Use native `<details>` / `<summary>` disclosures for non-essential edge cases, exhaustive option lists, and advanced configuration. Keep required steps, security warnings, destructive effects, and eligibility limits visible.
   - Use tables only for genuine comparisons, not narrative text.
8. Preserve internal links inside `docs/public/**` where possible. Link to source-only raw docs only when the raw note is intentionally not published.
9. Note docs impact and the page's primary content type in the PR body.

## Diagrams for Public Docs

When a page explains architecture, lifecycle, data flow, state, trust
boundaries, ownership, or a multi-step workflow, decide whether a visual
teaches more than prose, a table, or bullets. If it does, use
`/diagram-design` and load its
`references/kandev-public-docs.md` integration guide.

- Choose a semantic pattern first when behavior, state, ownership, trust, or
  risk carries the meaning. Then choose and load the nearest visual-type
  reference.
- Use `doc-inline`, `balanced`, and `mixed` for normal docs-column figures
  unless the page or source requires another output dial.
- Author a self-contained HTML source, run the diagram self-check, geometry
  check, and skin check, then export a reviewed local SVG. Use PNG only when a
  raster fallback is required. Store the published image under
  `docs/screenshots/` and reference it relatively.
- If labels are dense at docs-column width, tighten the SVG viewBox and raise
  the readable type ramp before publishing. Use a plain Markdown image so the
  landing publisher copies it to `/docs/screenshots`; do not nest it inside a
  Markdown link. Add a separate reference-style Markdown link targeting
  `../../docs/screenshots/<file>.svg` so readers can open the full-size vector.
- Give every image precise alt text and explain the diagram's essential result
  in nearby prose. Use real Kandev names from authoritative source material.
- For an existing Mermaid diagram, use the skill's Mermaid import workflow
  before revising it. Redraw for quality instead of reproducing Mermaid's
  automatic layout, and keep Mermaid only when the publication constraints
  make it the better source.
- Keep the diagram within its complexity budget. Split an overview from detail
  when the reader needs more than one focused figure.

## Validation

Run the checks relevant to your change:

```bash
# Replace SEARCH_TERM with the command, config key, or terminology that changed.
rg -n "SEARCH_TERM" docs/public docs/specs docs/decisions
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

For website docs publishing changes, also run from the landing repo:

```bash
pnpm install --frozen-lockfile
pnpm --filter @kandev/docs fetch-docs
pnpm exec vitest run apps/docs/lib/docs-processing.test.ts apps/docs/lib/public-docs.test.ts
pnpm --filter @kandev/docs build
```

## Final Report

State one of:

- `Public docs updated:` with changed `docs/public/**` files.
- `Internal docs updated:` with changed specs/plans/decisions.
- `No docs change needed:` with one concrete reason.
