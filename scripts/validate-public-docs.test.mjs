import assert from "node:assert/strict";
import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { validatePublicDocs } from "./validate-public-docs.mjs";

async function createDocs(files, meta) {
  const dir = await fs.mkdtemp(path.join(os.tmpdir(), "kandev-public-docs-"));
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
