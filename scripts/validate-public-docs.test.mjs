import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test, { after } from "node:test";
import {
  validateCoverageInventory,
  validateFeatureMedia,
  validatePublicDocs,
} from "./validate-public-docs.mjs";

const tempDirs = [];

/**
 * Create an isolated published-docs fixture.
 *
 * @param {Record<string, string>} files Fixture files keyed by relative path.
 * @param {{pages: string[]}} meta Navigation metadata.
 * @returns {Promise<string>} Temporary fixture directory.
 */
async function createDocs(files, meta) {
  const dir = await fs.mkdtemp(path.join(os.tmpdir(), "kandev-public-docs-"));
  tempDirs.push(dir);
  await Promise.all(
    Object.entries(files).map(async ([file, content]) => {
      const target = path.join(dir, file);
      await fs.mkdir(path.dirname(target), { recursive: true });
      await fs.writeFile(target, content);
    }),
  );
  await fs.writeFile(path.join(dir, "meta.json"), JSON.stringify(meta));
  return dir;
}

/**
 * Create a minimal repository fixture for source-backed coverage validation.
 *
 * @param {object} options Fixture overrides.
 * @returns {Promise<{repoRoot: string, docsDir: string}>} Fixture paths.
 */
async function createCoverageRepo({
  coverage,
  settingsRoutes = ["/settings", "/settings/general"],
  mcpTools = ["list_workspaces_kandev"],
  omitFiles = [],
} = {}) {
  const repoRoot = await fs.mkdtemp(
    path.join(os.tmpdir(), "kandev-doc-coverage-"),
  );
  tempDirs.push(repoRoot);
  const docsDir = path.join(repoRoot, "docs/public");
  const files = {
    "docs/public/index.md": validPage,
    "apps/web/src/settings-routes.tsx": `const SETTINGS_ROUTES = {\n${settingsRoutes
      .map((route) => `  "${route}": () => null,`)
      .join("\n")}\n};\n\nexport function SettingsRoutes() {}`,
    "apps/web/src/settings-routes.test.ts": "// settings route coverage",
    "apps/backend/internal/mcp/server/server.go": mcpTools
      .map((tool) => `mcp.NewTool("${tool}")`)
      .join("\n"),
    "apps/backend/internal/mcp/server/server_test.go": "// MCP coverage",
  };

  for (const [file, content] of Object.entries(files)) {
    if (omitFiles.includes(file)) continue;
    const target = path.join(repoRoot, file);
    await fs.mkdir(path.dirname(target), { recursive: true });
    await fs.writeFile(target, content);
  }

  const inventory = coverage ?? {
    version: 1,
    areas: [
      {
        id: "workspace-control",
        audiences: ["user"],
        stability: "stable",
        docs: ["index"],
        sources: [
          "apps/web/src/settings-routes.tsx",
          "apps/backend/internal/mcp/server/server.go",
        ],
        tests: [
          "apps/web/src/settings-routes.test.ts",
          "apps/backend/internal/mcp/server/server_test.go",
        ],
        settingsRoutes: ["/settings/general"],
        mcpTools: ["list_workspaces_kandev"],
      },
    ],
    exclusions: {
      settingsRoutes: [
        { route: "/settings", reason: "Alias for General settings." },
      ],
      mcpTools: [],
    },
  };
  await fs.writeFile(
    path.join(docsDir, "coverage.json"),
    JSON.stringify(inventory),
  );

  return { repoRoot, docsDir };
}

/**
 * Create a complete focused-media fixture with one embedded clip.
 *
 * @param {object} options Fixture overrides.
 * @returns {Promise<{docsDir: string, manifest: object, mediaDir: string}>}
 */
async function createFeatureMedia({ manifestPatch, extraFiles = {} } = {}) {
  const docsDir = await fs.mkdtemp(path.join(os.tmpdir(), "kandev-doc-media-"));
  tempDirs.push(docsDir);
  const mediaDir = path.join(docsDir, "media/feature-guides");
  await fs.mkdir(mediaDir, { recursive: true });

  const files = {
    "review.webm": "vp9-video",
    "review.mp4": "h264-video",
    "review.webp": "poster-image",
    ...extraFiles,
  };
  await Promise.all(
    Object.entries(files).map(([file, content]) =>
      fs.writeFile(path.join(mediaDir, file), content),
    ),
  );
  await fs.writeFile(
    path.join(mediaDir, "NOTES.txt"),
    "Isolated capture and QA notes.\n",
  );
  await fs.writeFile(
    path.join(docsDir, "review.md"),
    `${validPage.replace("# Kandev", "# Review")}\n## Review a diff\n\n<DocsVideo\n  webm="./media/feature-guides/review.webm"\n  mp4="./media/feature-guides/review.mp4"\n  poster="./media/feature-guides/review.webp"\n  title="Review a focused diff"\n/>\n`,
  );

  const fileRecord = (file, codec) => ({
    bytes: Buffer.byteLength(files[file]),
    codec,
    sha256: createHash("sha256").update(files[file]).digest("hex"),
  });
  const manifest = {
    schema_version: 1,
    generated_at: "2026-07-16T00:00:00.000Z",
    qa_status: "accepted",
    delivery_contract: {
      dimensions: { width: 960, height: 600 },
      frame_rate: 25,
      audio: false,
      video_formats: ["vp9-webm", "h264-mp4"],
      poster_format: "webp",
    },
    clips: [
      {
        slug: "review",
        title: "Review a focused diff",
        intended_docs: { page: "review.md", section: "Review a diff" },
        accessible_caption: "Select a changed line and send focused feedback.",
        duration_seconds: 8.4,
        dimensions: { width: 960, height: 600 },
        source_scenario:
          "Review an isolated fixture diff and leave line-level feedback.",
        data_isolation:
          "Disposable E2E workspace with mock-only agent data and no credentials.",
        filenames: {
          webm: "review.webm",
          mp4: "review.mp4",
          poster: "review.webp",
        },
        files: {
          webm: fileRecord("review.webm", "vp9"),
          mp4: fileRecord("review.mp4", "h264"),
          poster: fileRecord("review.webp", "webp"),
        },
      },
    ],
    ...manifestPatch,
  };
  await fs.writeFile(
    path.join(mediaDir, "manifest.json"),
    `${JSON.stringify(manifest, null, 2)}\n`,
  );
  return { docsDir, manifest, mediaDir };
}

after(async () => {
  await Promise.all(
    tempDirs.map((dir) => fs.rm(dir, { recursive: true, force: true })),
  );
});

const validPage = `---
title: "Overview"
description: "Start using Kandev."
---

# Kandev

Page body.
`;

test("accepts explicitly ordered pages with required frontmatter", async () => {
  const dir = await createDocs(
    {
      "README.md": "# Contributing",
      "index.md": validPage,
      "cli.md": validPage,
    },
    { title: "Kandev Docs", pages: ["---Start---", "index", "cli"] },
  );

  await assert.doesNotReject(validatePublicDocs(dir));
});

test("rejects published pages omitted from meta.json", async () => {
  const dir = await createDocs(
    { "index.md": validPage, "cli.md": validPage },
    { pages: ["index"] },
  );

  await assert.rejects(
    validatePublicDocs(dir),
    /meta.json is missing published page: cli/,
  );
});

test("rejects meta.json entries without a matching file", async () => {
  const dir = await createDocs(
    { "index.md": validPage },
    { pages: ["index", "nonexistent"] },
  );

  await assert.rejects(
    validatePublicDocs(dir),
    /meta.json references unknown page: nonexistent/,
  );
});

test("rejects unsupported link decorations as unknown pages", async () => {
  const dir = await createDocs(
    { "index.md": validPage },
    { pages: ["index", "external:[Support](https://example.com)"] },
  );

  await assert.rejects(
    validatePublicDocs(dir),
    /meta.json references unknown page: external:\[Support\]\(https:\/\/example.com\)/,
  );
});

test("rejects duplicate entries in meta.json", async () => {
  const dir = await createDocs(
    { "index.md": validPage },
    { pages: ["index", "index"] },
  );

  await assert.rejects(
    validatePublicDocs(dir),
    /meta.json lists page more than once: index/,
  );
});

test("rejects files that resolve to the same published slug", async () => {
  const dir = await createDocs(
    { "foo.md": validPage, "foo/index.md": validPage },
    { pages: ["foo"] },
  );

  await assert.rejects(
    validatePublicDocs(dir),
    /multiple published files resolve to slug foo: foo.md, foo\/index.md/,
  );
});

test("accepts single-character frontmatter values", async () => {
  const dir = await createDocs(
    {
      "index.md": `---
title: x
description: y
---

# X
`,
    },
    { pages: ["index"] },
  );

  await assert.doesNotReject(validatePublicDocs(dir));
});

test("rejects unsupported page status frontmatter", async () => {
  const dir = await createDocs(
    {
      "index.md": validPage.replace(
        'description: "Start using Kandev."',
        'description: "Start using Kandev."\nstatus: beta',
      ),
    },
    { pages: ["index"] },
  );

  await assert.rejects(
    validatePublicDocs(dir),
    /index.md has unsupported page status: beta/,
  );
});

test("accepts experimental page status frontmatter", async () => {
  const dir = await createDocs(
    {
      "index.md": validPage.replace(
        'description: "Start using Kandev."',
        'description: "Start using Kandev."\nstatus: experimental',
      ),
    },
    { pages: ["index"] },
  );

  await assert.doesNotReject(validatePublicDocs(dir));
});

test("rejects an experimental warning as the only content in a status section", async () => {
  const dir = await createDocs(
    {
      "index.md": `${validPage}\n## Office status\n\n> [!EXPERIMENTAL]\n> Office is disabled by default.\n`,
    },
    { pages: ["index"] },
  );

  await assert.rejects(
    validatePublicDocs(dir),
    /index.md has a warning-only experimental status section: Office status/,
  );
});

test("accepts an experimental status section with substantive content", async () => {
  const dir = await createDocs(
    {
      "index.md": `${validPage}\n## Office status\n\n> [!EXPERIMENTAL]\n> Office is disabled by default.\n\nUse workflow approval steps for the supported human-gated path.\n`,
    },
    { pages: ["index"] },
  );

  await assert.doesNotReject(validatePublicDocs(dir));
});

test("rejects published pages without title and description frontmatter", async () => {
  const dir = await createDocs(
    { "index.md": "# Kandev\n" },
    { pages: ["index"] },
  );

  await assert.rejects(
    validatePublicDocs(dir),
    /index.md must start with YAML frontmatter containing title and description/,
  );
});

test("rejects a published page without a level-one content heading", async () => {
  const dir = await createDocs(
    {
      "index.md": `---
title: "Overview"
description: "Start using Kandev."
---

Start here.
`,
    },
    { pages: ["index"] },
  );

  await assert.rejects(
    validatePublicDocs(dir),
    /index.md must begin with exactly one level-one heading after frontmatter/,
  );
});

test("rejects a second level-one content heading", async () => {
  const dir = await createDocs(
    { "index.md": `${validPage}\n# Another page title\n` },
    { pages: ["index"] },
  );

  await assert.rejects(
    validatePublicDocs(dir),
    /index.md must begin with exactly one level-one heading after frontmatter/,
  );
});

test("ignores level-one headings shown inside code examples", async () => {
  const dir = await createDocs(
    {
      "index.md": validPage.replace(
        "Page body.",
        "```markdown\n# Example document\n```",
      ),
    },
    { pages: ["index"] },
  );

  await assert.doesNotReject(validatePublicDocs(dir));
});

test("accepts existing relative page and asset links", async () => {
  const dir = await createDocs(
    {
      "index.md": validPage.replace(
        "Page body.",
        "[Guide](guide.md)\n\n![Diagram](assets/diagram.png)",
      ),
      "guide.md": validPage,
      "assets/diagram.png": "not-a-real-png",
    },
    { pages: ["index", "guide"] },
  );

  await assert.doesNotReject(validatePublicDocs(dir));
});

test("rejects a broken local page link", async () => {
  const dir = await createDocs(
    {
      "index.md": validPage.replace("Page body.", "[Missing](missing.md)"),
    },
    { pages: ["index"] },
  );

  await assert.rejects(
    validatePublicDocs(dir),
    /index.md links to missing local target: missing.md/,
  );
});

test("rejects a broken local image", async () => {
  const dir = await createDocs(
    {
      "index.md": validPage.replace(
        "Page body.",
        "![Missing](assets/missing.png)",
      ),
    },
    { pages: ["index"] },
  );

  await assert.rejects(
    validatePublicDocs(dir),
    /index.md links to missing local target: assets\/missing.png/,
  );
});

test("accepts existing DocsVideo sources and poster assets", async () => {
  const dir = await createDocs(
    {
      "index.md": validPage.replace(
        "Page body.",
        `<DocsVideo
  webm="./media/review.webm"
  mp4="./media/review.mp4"
  poster="./media/review.webp"
  title="Review changes"
/>`,
      ),
      "media/review.webm": "webm",
      "media/review.mp4": "mp4",
      "media/review.webp": "webp",
    },
    { pages: ["index"] },
  );

  await assert.doesNotReject(validatePublicDocs(dir));
});

test("rejects a DocsVideo attribute that points to missing media", async () => {
  const dir = await createDocs(
    {
      "index.md": validPage.replace(
        "Page body.",
        `<DocsVideo
  mp4="./media/missing.mp4"
  poster="./media/review.webp"
  title="Review changes"
/>`,
      ),
      "media/review.webp": "webp",
    },
    { pages: ["index"] },
  );

  await assert.rejects(
    validatePublicDocs(dir),
    /index.md links to missing local target: \.\/media\/missing\.mp4/,
  );
});

test("rejects a broken local image nested inside a link", async () => {
  const dir = await createDocs(
    {
      "index.md": validPage.replace(
        "Page body.",
        "[![Missing](assets/missing.png)](guide.md)",
      ),
      "guide.md": validPage,
    },
    { pages: ["index", "guide"] },
  );

  await assert.rejects(
    validatePublicDocs(dir),
    /index.md links to missing local target: assets\/missing.png/,
  );
});

test("accepts external, existing heading, and fenced-code links", async () => {
  const dir = await createDocs(
    {
      "index.md": validPage.replace(
        "Page body.",
        `[External](https://example.com/docs)\n\n## Local section\n\n[Section](#local-section)\n\n[Other section](guide.md#nested-options)\n\n\`\`\`md
[Example only](does-not-exist.md)
\`\`\``,
      ),
      "guide.md": validPage.replace("Page body.", "## Nested `options`\n"),
    },
    { pages: ["index", "guide"] },
  );

  await assert.doesNotReject(validatePublicDocs(dir));
});

test("rejects a missing same-page heading fragment", async () => {
  const dir = await createDocs(
    {
      "index.md": validPage.replace(
        "Page body.",
        "[Missing section](#missing-section)",
      ),
    },
    { pages: ["index"] },
  );

  await assert.rejects(
    validatePublicDocs(dir),
    /index.md links to missing heading: #missing-section/,
  );
});

test("rejects a missing heading fragment in another page", async () => {
  const dir = await createDocs(
    {
      "index.md": validPage.replace(
        "Page body.",
        "[Missing section](guide.md#missing-section)",
      ),
      "guide.md": validPage,
    },
    { pages: ["index", "guide"] },
  );

  await assert.rejects(
    validatePublicDocs(dir),
    /index.md links to missing heading: guide.md#missing-section/,
  );
});

test("accepts heading fragments whose labels contain complete inline HTML", async () => {
  const dir = await createDocs(
    {
      "index.md": validPage.replace(
        "Page body.",
        "## Configure <span>profiles</span>\n\n[Profiles](#configure-profiles)",
      ),
    },
    { pages: ["index"] },
  );

  await assert.doesNotReject(validatePublicDocs(dir));
});

test("rejects unterminated inline HTML in a linked heading", async () => {
  const dir = await createDocs(
    {
      "index.md": validPage.replace(
        "Page body.",
        "## Unsafe <script\n\n[Unsafe](#unsafe-script)",
      ),
    },
    { pages: ["index"] },
  );

  await assert.rejects(
    validatePublicDocs(dir),
    /heading contains unterminated inline HTML/,
  );
});

test("ignores links inside inline code and nested shorter fences", async () => {
  const dir = await createDocs(
    {
      "index.md": validPage.replace(
        "Page body.",
        `Use \`[example](inline-missing.md)\` in prose.

\`\`\`\`md
\`\`\`md
[Example only](fenced-missing.md)
\`\`\`
\`\`\`\``,
      ),
    },
    { pages: ["index"] },
  );

  await assert.doesNotReject(validatePublicDocs(dir));
});

test("ignores links inside indented code blocks", async () => {
  const dir = await createDocs(
    {
      "index.md": validPage.replace(
        "Page body.",
        `Example only:

    [Missing](indented-missing.md)`,
      ),
    },
    { pages: ["index"] },
  );

  await assert.doesNotReject(validatePublicDocs(dir));
});

test("ignores indented code blocks after a heading", async () => {
  const dir = await createDocs(
    {
      "index.md": validPage.replace(
        "Page body.",
        `## Example
    [Missing](heading-code-missing.md)`,
      ),
    },
    { pages: ["index"] },
  );

  await assert.doesNotReject(validatePublicDocs(dir));
});

test("ignores indented code blocks nested in list items", async () => {
  const dir = await createDocs(
    {
      "index.md": validPage.replace(
        "Page body.",
        `- Example only:

      [Missing](list-code-missing.md)`,
      ),
    },
    { pages: ["index"] },
  );

  await assert.doesNotReject(validatePublicDocs(dir));
});

test("validates links in indented list paragraphs", async () => {
  const dir = await createDocs(
    {
      "index.md": validPage.replace(
        "Page body.",
        `- Related material:

    [Missing](list-paragraph-missing.md)`,
      ),
    },
    { pages: ["index"] },
  );

  await assert.rejects(
    validatePublicDocs(dir),
    /index.md links to missing local target: list-paragraph-missing.md/,
  );
});

test("accepts escaped parentheses in local link destinations", async () => {
  const dir = await createDocs(
    {
      "index.md": validPage.replace("Page body.", "[Guide](guide\\(1\\).md)"),
      "guide(1).md": validPage,
    },
    { pages: ["index", "guide(1)"] },
  );

  await assert.doesNotReject(validatePublicDocs(dir));
});

test("accepts balanced parentheses in local link destinations", async () => {
  const dir = await createDocs(
    {
      "index.md": validPage.replace("Page body.", "[Guide](guide(1).md)"),
      "guide(1).md": validPage,
    },
    { pages: ["index", "guide(1)"] },
  );

  await assert.doesNotReject(validatePublicDocs(dir));
});

test("validates reference-style link definitions", async () => {
  const dir = await createDocs(
    {
      "index.md": validPage.replace(
        "Page body.",
        "[Guide][user guide]\n\n[user guide]: missing.md",
      ),
    },
    { pages: ["index"] },
  );

  await assert.rejects(
    validatePublicDocs(dir),
    /index.md links to missing local target: missing.md/,
  );
});

test("rejects undefined full reference-style links", async () => {
  const dir = await createDocs(
    {
      "index.md": validPage.replace("Page body.", "[Guide][missing guide]"),
    },
    { pages: ["index"] },
  );

  await assert.rejects(
    validatePublicDocs(dir),
    /index.md uses undefined Markdown reference: missing guide/,
  );
});

test("validates collapsed reference-style links", async () => {
  const dir = await createDocs(
    {
      "index.md": validPage.replace(
        "Page body.",
        "[Guide][]\n\n[guide]: missing.md",
      ),
    },
    { pages: ["index"] },
  );

  await assert.rejects(
    validatePublicDocs(dir),
    /index.md links to missing local target: missing.md/,
  );
});

test("rejects undefined shortcut reference-style links", async () => {
  const dir = await createDocs(
    {
      "index.md": validPage.replace("Page body.", "Read the [Guide]."),
    },
    { pages: ["index"] },
  );

  await assert.rejects(
    validatePublicDocs(dir),
    /index.md uses undefined Markdown reference: guide/,
  );
});

test("ignores non-reference bracket syntax", async () => {
  const dir = await createDocs(
    {
      "index.md": validPage.replace(
        "Page body.",
        "> [!WARNING]\n\n- [x] Done\n\n[^note]\n\n\\[Literal]",
      ),
    },
    { pages: ["index"] },
  );

  await assert.doesNotReject(validatePublicDocs(dir));
});

test("checks local links in README files", async () => {
  const dir = await createDocs(
    {
      "README.md": "# Contributing\n\n[Missing](missing.md)",
      "index.md": validPage,
    },
    { pages: ["index"] },
  );

  await assert.rejects(
    validatePublicDocs(dir),
    /README.md links to missing local target: missing.md/,
  );
});

test("rejects site-root links because public docs use relative sources", async () => {
  const dir = await createDocs(
    {
      "index.md": validPage.replace("Page body.", "[Guide](/docs/guide)"),
    },
    { pages: ["index"] },
  );

  await assert.rejects(
    validatePublicDocs(dir),
    /index.md uses a site-root link instead of a relative source link: \/docs\/guide/,
  );
});

test("accepts source-backed coverage for every settings route and MCP tool", async () => {
  const fixture = await createCoverageRepo();

  await assert.doesNotReject(validateCoverageInventory(fixture));
});

test("rejects a shipped settings route with no documentation owner", async () => {
  const fixture = await createCoverageRepo({
    settingsRoutes: [
      "/settings",
      "/settings/general",
      "/settings/system/status",
    ],
  });

  await assert.rejects(
    validateCoverageInventory(fixture),
    /coverage.json does not account for settings route: \/settings\/system\/status/,
  );
});

test("rejects a registered MCP tool with no documentation owner", async () => {
  const fixture = await createCoverageRepo({
    mcpTools: ["list_workspaces_kandev", "create_task_kandev"],
  });

  await assert.rejects(
    validateCoverageInventory(fixture),
    /coverage.json does not account for MCP tool: create_task_kandev/,
  );
});

test("rejects coverage entries that cite missing evidence", async () => {
  const fixture = await createCoverageRepo({
    omitFiles: ["apps/web/src/settings-routes.test.ts"],
  });

  await assert.rejects(
    validateCoverageInventory(fixture),
    /workspace-control cites missing test: apps\/web\/src\/settings-routes\.test\.ts/,
  );
});

test("rejects coverage evidence that points to a directory", async () => {
  const fixture = await createCoverageRepo();
  const coveragePath = path.join(fixture.docsDir, "coverage.json");
  const coverage = JSON.parse(await fs.readFile(coveragePath, "utf8"));
  coverage.areas[0].tests[0] = "apps/web/src";
  await fs.writeFile(coveragePath, JSON.stringify(coverage));

  await assert.rejects(
    validateCoverageInventory(fixture),
    /workspace-control cites non-file test: apps\/web\/src/,
  );
});

test("rejects shipped surfaces listed as both covered and excluded", async () => {
  const fixture = await createCoverageRepo();
  const coveragePath = path.join(fixture.docsDir, "coverage.json");
  const coverage = JSON.parse(await fs.readFile(coveragePath, "utf8"));
  coverage.exclusions.settingsRoutes.push({
    route: "/settings/general",
    reason: "This deliberately overlaps the covered route.",
  });
  await fs.writeFile(coveragePath, JSON.stringify(coverage));

  await assert.rejects(
    validateCoverageInventory(fixture),
    /coverage.json both covers and excludes settings route: \/settings\/general/,
  );
});

test("rejects duplicate coverage area identifiers", async () => {
  const sharedArea = {
    id: "workspace-control",
    audiences: ["user"],
    stability: "stable",
    docs: ["index"],
    sources: ["apps/web/src/settings-routes.tsx"],
    tests: ["apps/web/src/settings-routes.test.ts"],
    settingsRoutes: ["/settings/general"],
    mcpTools: ["list_workspaces_kandev"],
  };
  const fixture = await createCoverageRepo({
    coverage: {
      version: 1,
      areas: [sharedArea, { ...sharedArea }],
      exclusions: {
        settingsRoutes: [
          { route: "/settings", reason: "Alias for General settings." },
        ],
        mcpTools: [],
      },
    },
  });

  await assert.rejects(
    validateCoverageInventory(fixture),
    /coverage.json repeats area id: workspace-control/,
  );
});

test("rejects a published page with no feature coverage owner", async () => {
  const { repoRoot, docsDir } = await createCoverageRepo();
  await fs.writeFile(path.join(docsDir, "security.md"), validPage);

  await assert.rejects(
    validateCoverageInventory({ repoRoot, docsDir }),
    /coverage.json does not account for docs page: security/,
  );
});

test("accepts reviewed focused media with matching files and documentation owner", async () => {
  const { docsDir } = await createFeatureMedia();

  await assert.doesNotReject(validateFeatureMedia({ docsDir }));
});

test("rejects focused media whose QA status is not accepted", async () => {
  const { docsDir } = await createFeatureMedia({
    manifestPatch: { qa_status: "rejected" },
  });

  await assert.rejects(
    validateFeatureMedia({ docsDir }),
    /feature media manifest must have accepted QA status/,
  );
});

test("rejects focused media without durable capture provenance", async () => {
  const { docsDir, manifest, mediaDir } = await createFeatureMedia();
  delete manifest.clips[0].data_isolation;
  await fs.writeFile(
    path.join(mediaDir, "manifest.json"),
    `${JSON.stringify(manifest, null, 2)}\n`,
  );

  await assert.rejects(
    validateFeatureMedia({ docsDir }),
    /feature media clip review has invalid data_isolation/,
  );
});

test("rejects focused media whose content no longer matches its hash", async () => {
  const { docsDir, mediaDir } = await createFeatureMedia();
  await fs.writeFile(path.join(mediaDir, "review.mp4"), "h264-videX");

  await assert.rejects(
    validateFeatureMedia({ docsDir }),
    /review\.mp4 SHA-256 does not match manifest/,
  );
});

test("rejects a focused-media target section that is absent from the page", async () => {
  const { docsDir, manifest, mediaDir } = await createFeatureMedia();
  manifest.clips[0].intended_docs.section = "Missing workflow";
  await fs.writeFile(
    path.join(mediaDir, "manifest.json"),
    `${JSON.stringify(manifest, null, 2)}\n`,
  );

  await assert.rejects(
    validateFeatureMedia({ docsDir }),
    /review targets missing section in review\.md: Missing workflow/,
  );
});

test("rejects focused media that is not embedded on its intended page", async () => {
  const { docsDir } = await createFeatureMedia();
  await fs.writeFile(
    path.join(docsDir, "review.md"),
    `${validPage}\n## Review a diff\n\nNo video is embedded here.\n`,
  );

  await assert.rejects(
    validateFeatureMedia({ docsDir }),
    /review is not embedded with its complete media triplet in review\.md/,
  );
});

test("requires each focused-media file in its matching DocsVideo attribute", async () => {
  const { docsDir } = await createFeatureMedia();
  await fs.writeFile(
    path.join(docsDir, "review.md"),
    `${validPage}\n## Review a diff\n\n<DocsVideo
  webm="./media/feature-guides/review.webm"
  poster="./media/feature-guides/review.webp"
  title="media/feature-guides/review.mp4"
/>\n`,
  );

  await assert.rejects(
    validateFeatureMedia({ docsDir }),
    /review is not embedded with its complete media triplet in review\.md/,
  );
});

test("rejects orphaned focused-media deliverables", async () => {
  const { docsDir } = await createFeatureMedia({
    extraFiles: { "orphan.webp": "unused-poster" },
  });

  await assert.rejects(
    validateFeatureMedia({ docsDir }),
    /feature media directory contains untracked file: orphan\.webp/,
  );
});
