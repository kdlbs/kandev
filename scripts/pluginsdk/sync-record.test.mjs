import assert from "node:assert/strict";
import { mkdtemp, mkdir, readFile, writeFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { syncRecord } from "./sync-record.mjs";

// @covers AC-PLUGINS-SDK-DISTRIBUTION-002.1
test("synchronizes canonical metadata and detects drift without rewriting in check mode", async () => {
  const root = await mkdtemp(join(tmpdir(), "sdk-record-"));
  try {
    await mkdir(join(root, "pluginsdk"));
    await mkdir(join(root, "apps/packages/plugin-sdk"), { recursive: true });
    const record = {
      schema_version: 1,
      sdk_version: "0.1.0",
      host_release: null,
      min_kandev_version: null,
      api_versions: [1, 2],
      change_kind: "initial",
    };
    const canonical = JSON.stringify(record, null, 2) + "\n";
    const copy = join(root, "apps/packages/plugin-sdk/sdk-release.json");
    await writeFile(join(root, "pluginsdk/sdk-release.json"), canonical);
    await writeFile(
      join(root, "apps/packages/plugin-sdk/package.json"),
      JSON.stringify({ version: "0.1.0" }),
    );
    await writeFile(copy, "{}\n");
    await syncRecord(root);
    assert.equal(await readFile(copy, "utf8"), canonical);
    await syncRecord(root, true);
    await writeFile(copy, "{}\n");
    await assert.rejects(syncRecord(root, true), /release record differs/);
    assert.equal(await readFile(copy, "utf8"), "{}\n");
    await writeFile(
      join(root, "apps/packages/plugin-sdk/package.json"),
      JSON.stringify({ version: "0.2.0" }),
    );
    await assert.rejects(syncRecord(root), /package version/);
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});
