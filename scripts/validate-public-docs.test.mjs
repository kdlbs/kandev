import assert from "node:assert/strict";
import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test, { after } from "node:test";
import { validatePublicDocs } from "./validate-public-docs.mjs";

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
