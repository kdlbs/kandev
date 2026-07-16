# Contributing to the Public Docs

Files in this directory are the source for [kandev.ai/docs](https://kandev.ai/docs). A merged change under `docs/public/**` triggers a Cloudflare Pages rebuild automatically.

The docs have two audiences:

- **Use Kandev** explains product tasks, configuration, security implications, limitations, and operations.
- **Contribute to Kandev** explains repository structure, code boundaries, testing, extension points, releases, and docs maintenance.

Keep a page focused on one audience. Link across the boundary when a user-facing explanation needs contributor detail, but do not mix internal implementation notes into a task guide.

## Update an Existing Page

Edit the relevant Markdown file directly. Verify commands, configuration keys, default values, API names, labels, and screenshots against the implementation in the same pull request.

Use relative links between published pages, for example `[CLI](cli.md)`. The website build rewrites those links to `/docs/...` routes. Links to repository files outside `docs/public` are rewritten to their GitHub source URLs.

When behavior is conditional, say whether it is:

- supported by default;
- optional and dependent on an executor, integration, platform, or credential;
- experimental or feature-flagged;
- in progress or internal.

Do not describe an ADR, spec, plan, hidden route, or feature flag as generally available without confirming its runtime and UI path.

## Add a Page

1. Decide whether the page belongs under **Use Kandev** or **Contribute to Kandev**. Search existing pages first; prefer extending one task guide over creating overlapping reference pages.
2. Create a lowercase, kebab-case Markdown file in this directory, such as `custom-executors.md`. Its filename becomes its stable route: `/docs/custom-executors`.
3. Start the file with non-empty `title` and `description` frontmatter, followed by one level-one heading:

   ```markdown
   ---
   title: "Custom Executors"
   description: "Configure a custom executor for Kandev tasks."
   ---

   # Custom Executors

   Page content starts here.
   ```

4. Add the filename without `.md` to `meta.json` exactly once, below the correct audience heading. Its position controls sidebar order. Keep routes flat unless a concrete publication constraint requires a directory.
5. Link the new page from `index.md` and from closely related guides. Use relative Markdown links and repository-owned assets.
6. Include prerequisites, a concrete workflow, exact configuration, limitations or failure modes, and related pages where those sections apply.

Pages omitted from `meta.json`, duplicate entries, unknown entries, broken local Markdown links, and missing frontmatter fail validation.

## Assets and Diagrams

- Keep user-facing screenshots under `docs/screenshots/` and reference them relatively.
- Do not link temporary capture output, local paths, or third-party hotlinked images.
- Mermaid code fences render on the website. Keep diagrams readable at docs-column width, use stable node labels, and provide surrounding prose that carries the same essential meaning.
- Prefer commands and text examples over screenshots when the UI changes frequently.

## Validate

Run the focused dependency-free docs checks from the Kandev repository root:

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

The broader repository script suite requires the pinned repository toolchain and installed dependencies:

```bash
make test-scripts
```

To build the complete docs site, clone `kdlbs/landing` beside this repository and run with the toolchain expected by Landing:

```bash
cd ../landing
pnpm install --frozen-lockfile
KANDEV_DOCS_SOURCE_PATH=../kandev/docs pnpm --filter @kandev/docs build
```

Before opening a pull request, also check:

- every new or renamed page appears once in `meta.json`;
- links resolve from the source file, not from the repository root;
- user claims are supported by current code or tests;
- feature status and platform limitations are explicit;
- generated files under the Landing docs app were not committed.
