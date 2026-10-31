import assert from "node:assert/strict";
import { mkdtemp, mkdir, readFile, readdir, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { randomBytes } from "node:crypto";
import { gunzipSync } from "node:zlib";
import { test } from "vitest";
import { precompressAssets } from "./precompress-assets.mjs";

test("compressed build assets are deterministic, smaller, and byte-exact", async (t) => {
  const root = await mkdtemp(path.join(os.tmpdir(), "kandev-gzip-"));
  t.onTestFinished(() => rm(root, { recursive: true, force: true }));
  const out = path.join(root, "custom-output", "assets");
  await mkdir(out, { recursive: true });
  const source = Buffer.from("export const repeated = true;\n".repeat(200));
  for (const name of ["chunk.js", "style.css", "index.html", "chunk.js.map", "image.svg"])
    await writeFile(path.join(out, name), source);
  await writeFile(path.join(out, "tiny.js"), "short");
  await writeFile(path.join(out, "random.js"), randomBytes(4096));
  await precompressAssets(path.dirname(out));
  for (const name of ["chunk.js", "style.css"]) {
    const compressed = await readFile(path.join(out, `${name}.gz`));
    assert.ok(compressed.length < source.length);
    assert.deepEqual(gunzipSync(compressed), source);
    assert.deepEqual(await readFile(path.join(out, name)), source);
    await precompressAssets(path.dirname(out));
    assert.deepEqual(await readFile(path.join(out, `${name}.gz`)), compressed);
  }
  assert.deepEqual((await readdir(out)).filter((name) => name.endsWith(".gz")).sort(), [
    "chunk.js.gz",
    "style.css.gz",
  ]);
  await writeFile(path.join(out, "chunk.js"), "now small");
  await precompressAssets(path.dirname(out));
  assert.ok(
    !(await readdir(out)).includes("chunk.js.gz"),
    "a previous compressed representation must not survive changed output",
  );
});

test("a missing build output rejects instead of silently skipping compression", async () => {
  await assert.rejects(precompressAssets(path.join(os.tmpdir(), `missing-kandev-${process.pid}`)), {
    code: "ENOENT",
  });
});
