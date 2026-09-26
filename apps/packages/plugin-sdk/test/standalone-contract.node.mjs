import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const packageURL = new URL("../package.json", import.meta.url);
const sourceURL = new URL("../src/index.ts", import.meta.url);

test("the public contract has no external package dependencies", async () => {
  const manifest = JSON.parse(await readFile(packageURL, "utf8"));

  assert.deepEqual(manifest.dependencies ?? {}, {});
  assert.deepEqual(manifest.peerDependencies ?? {}, {});
});

test("the public contract has no bare module imports", async () => {
  const source = await readFile(sourceURL, "utf8");
  const bareModuleImport = /(?:from\s+|import\s+)["'](?![./])/;

  assert.doesNotMatch(source, bareModuleImport);
});
