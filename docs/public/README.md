# Contributing to the Public Docs

`docs/public/` is the source for [kandev.ai/docs](https://kandev.ai/docs). Pull requests validate this tree. After a push to `main`, `.github/workflows/notify-docs.yml` validates it again and, when the deploy hook is configured, calls Landing; a validation failure prevents publication.

The published docs have two audiences:

- **Use Kandev** covers product tasks, configuration, limits, security, and operations.
- **Contribute to Kandev** covers code ownership, development, testing, extension points, releases, and docs maintenance.

Keep implementation detail in contributor pages. A user guide should explain the supported behavior and link here when internal context is useful.

## Update an existing page

Edit the Markdown source and verify every command, setting, label, default, platform claim, and screenshot against current source or tests. Preserve the filename when possible: it is the public slug.

Use source-relative links such as `[CLI](cli.md)`. Publication rewrites links between public pages to `/docs/...` and links to repository files outside `docs/public/` to GitHub. Do not use site-root links or local machine paths.

State whether behavior is default, optional, platform-dependent, experimental, feature-flagged, internal, or still in progress. An ADR, spec, hidden route, or source package is not evidence that a feature is generally available.

### Mark experimental content

Use page-level status when the whole guide describes an experimental surface:

```yaml
---
title: "Kubernetes"
description: "Deploy Kandev to Kubernetes."
status: experimental
---
```

The docs header renders a visible status callout from that frontmatter. To invoke this experimental page indicator, use the exact value `experimental`; omit `status` for stable pages. Source validation rejects other page-status values. The separate `stable`, `beta`, and `experimental` values in `coverage.json` describe inventory stability and do not create page badges.

Use a section-level indicator when a stable guide contains one experimental capability. Keep this source syntax as Markdown so older renderers and GitHub still show an explicit callout; the publication layer upgrades it to the styled status component:

```markdown
## Coordinate work

Use named sessions for parallel work that shares one task environment.

> [!EXPERIMENTAL]
> Office is disabled by default and its public workflow is not stable yet.
```

Place the indicator inside the substantive section it qualifies and explain the enabling condition, current limit, and supported alternative. Do not create a standalone status section whose only content is the warning. The publication pipeline converts the callout to `FeatureStatus` and opts the generated page into MDX automatically.

## Add a page

1. Search existing pages and choose **Use Kandev** or **Contribute to Kandev**.
2. Create a lowercase, kebab-case Markdown file in `docs/public/`. The filename becomes the stable route; `custom-executors.md` publishes as `/docs/custom-executors`.
3. Add non-empty `title` and `description` frontmatter, then one level-one heading:

   ```markdown
   ---
   title: "Custom Executors"
   description: "Configure a custom executor for Kandev tasks."
   ---

   # Custom Executors
   ```

4. Add the slug, without `.md`, exactly once to `docs/public/meta.json` under the correct audience heading. Its position controls sidebar order. `README.md` is the maintenance guide and is not a published page.
5. Add the page to at least one area in `docs/public/coverage.json`, with current implementation and test evidence.
6. Link the page from the relevant overview and neighboring guides.
7. Include prerequisites, a concrete workflow, exact configuration, failure modes or limits, and related pages where applicable.

The validator rejects missing frontmatter, pages absent from navigation or coverage, duplicate or unknown navigation entries, broken local files or heading fragments, missing assets, and site-root links.

## Maintain feature coverage

`docs/public/coverage.json` is the source-backed map from shipped product areas to their user, operator, contributor, or integrator documentation. Every area cites concrete source and test files. The validator also discovers every static Settings route and registered Kandev MCP tool, then requires each one to have exactly one coverage owner or an explicit exclusion with a reason.

When behavior changes:

1. Update the owning page and its support boundary.
2. Update the area's source or test evidence when ownership moved.
3. Assign a new Settings route or MCP tool to the appropriate area.
4. Add a new area only when the behavior has a distinct audience, stability, and documentation owner.

Do not use the inventory to claim that a route, package, ADR, test harness, or feature flag is generally available. The page must still explain whether the user-facing behavior is stable, dependency-bound, limited, experimental, or unavailable.

## Assets and diagrams

- Store product screenshots under `docs/screenshots/` and reference them relatively.
- Do not hotlink mutable third-party images or commit temporary capture output.
- Mermaid diagrams are supported. Keep them readable at docs-column width and explain the essential result in prose.
- Prefer text and commands when the UI changes frequently.

Use a short focused video only when motion or interaction order communicates something that prose and a still image cannot. Store reviewed deliverables under `docs/public/media/feature-guides/` as a WebM, an H.264 MP4 fallback, and a WebP poster. Keep raw captures, browser profiles, and temporary encoder output outside the repository.

Embed the local sources with the publication component:

```mdx
<DocsVideo
  webm="./media/feature-guides/review.webm"
  mp4="./media/feature-guides/review.mp4"
  poster="./media/feature-guides/review.webp"
  title="Review a focused diff"
  caption="Inspect a changed line and leave feedback for the agent."
/>
```

Videos must show current isolated test data, avoid real credentials and production repositories, use native controls, and remain understandable from the poster and caption. Prefer a compact view that follows the relevant cursor or touch target over a wide application recording. Keep `manifest.json` and `NOTES.txt` beside the reviewed triplets. Source validation checks accepted QA status, the 960x600/25fps/no-audio delivery contract, page and section ownership, complete embeds, byte counts, SHA-256 hashes, and orphaned files. Do not add Markdown files under `docs/public/media/` because public Markdown is treated as a page.

## Validate the source

Run the dependency-free checks from the Kandev repository root:

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

`make test-scripts` runs the validator's unit tests among other repository script tests; it does not replace the live validation command above.

## Verify the Landing publication build

With `kdlbs/landing` checked out beside `kandev`, use Landing's documented Node 24.12.0 and pnpm 10.29.3 prerequisites and build the same root-plus-docs artifact deployed to Cloudflare Pages:

```bash
cd ../landing
pnpm install --frozen-lockfile
KANDEV_DOCS_SOURCE_PATH="$(cd ../kandev && pwd)/docs" pnpm build:pages
```

`KANDEV_DOCS_SOURCE_PATH` makes Landing fetch the local public pages instead of GitHub. Pass an absolute path because the docs package runs its prebuild from `landing/apps/docs`. The build copies only pages named by `meta.json`, rewrites links, and builds both the landing site and docs route. Do not commit generated Landing content, `.next`, or `out` directories.

Before review, confirm:

- each new or renamed page appears exactly once in `meta.json`;
- links resolve relative to their source file;
- claims are supported by current code, tests, or workflows;
- conditional support and platform limits are explicit;
- source validation and, for publication-sensitive changes, the Landing build pass.
