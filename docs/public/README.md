# Contributing to the Public Docs

Files in this directory are the source for [kandev.ai/docs](https://kandev.ai/docs). A merged change under `docs/public/**` triggers a Cloudflare Pages rebuild automatically.

## Update an Existing Page

Edit the relevant Markdown file directly. Keep commands, configuration keys, API names, and screenshots aligned with the implementation in the same pull request.

Use relative links between published pages, for example `[CLI](cli.md)`. The website build rewrites those links to `/docs/...` routes. Links to repository files outside `docs/public` are rewritten to their GitHub source URLs.

## Add a Page

1. Create a lowercase, kebab-case Markdown file in this directory, such as `custom-executors.md`. Its filename becomes its route: `custom-executors.md` is published at `/docs/custom-executors`.
2. Start the file with `title` and `description` frontmatter, followed by one level-one heading:

   ```markdown
   ---
   title: "Custom Executors"
   description: "Configure a custom executor for Kandev tasks."
   ---

   # Custom Executors

   Page content starts here.
   ```

3. Add the filename without its extension to `meta.json` exactly once. Its position controls sidebar order. Entries such as `---Execution---` create section labels.
4. Link the new page from related documentation where useful.

Pages omitted from `meta.json`, duplicate entries, unknown entries, and missing frontmatter fail validation.

## Validate

Run the dependency-free checks from the Kandev repository root:

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

To build the complete docs site, clone `kdlbs/landing` beside this repository and run:

```bash
cd ../landing
pnpm install --frozen-lockfile
KANDEV_DOCS_SOURCE_PATH=../kandev/docs pnpm --filter @kandev/docs build
```
