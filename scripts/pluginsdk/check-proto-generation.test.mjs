import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtemp, mkdir, writeFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { checkGeneration } from "./check-proto-generation.mjs";

// @covers AC-PLUGINS-SDK-DISTRIBUTION-004.1
for (const change of ["unchanged", "modified", "added", "removed"]) {
  test(`generation checks ${change} outputs, including untracked files`, async (t) => {
    const root = await mkdtemp(join(tmpdir(), "sdk-proto-test-"));
    t.after(() => rm(root, { recursive: true, force: true }));
    await mkdir(join(root, "proto"));
    const file = join(root, "proto/plugin.pb.go");
    await writeFile(file, "original");
    const generate = async () => {
      if (change === "modified") await writeFile(file, "changed");
      if (change === "added")
        await writeFile(join(root, "proto/new.pb.go"), "new");
      if (change === "removed") await rm(file);
    };
    if (change === "unchanged") await checkGeneration(root, generate);
    else
      await assert.rejects(
        checkGeneration(root, generate),
        /generated outputs differ/,
      );
  });
}
