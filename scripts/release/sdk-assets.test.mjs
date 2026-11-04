import test from "node:test";
import assert from "node:assert/strict";
import { mkdtemp, writeFile, readFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, basename } from "node:path";
import { ReleaseAssets } from "./sdk-assets.mjs";
async function fixture(t) {
  const root = await mkdtemp(join(tmpdir(), "sdk-assets-"));
  t.after(() => rm(root, { recursive: true, force: true }));
  const files = {};
  for (const key of ["archive", "receiptPath", "signaturePath"]) {
    files[key] = join(root, key);
    await writeFile(files[key], key);
  }
  const stored = new Map();
  let fail = false,
    uploads = 0;
  const run = async (cmd, args) => {
    if (args[0] === "api" && args[1].includes("/releases/tags/"))
      return {
        stdout: JSON.stringify({
          assets: [...stored].map(([name]) => ({ id: name, name })),
        }),
      };
    if (args[0] === "api")
      return { stdout: Buffer.from(stored.get(args[1].split("/").at(-1))) };
    if (args[0] === "release" && args[1] === "upload") {
      uploads++;
      assert.ok(
        !args[3].includes("#"),
        "asset names must come from filenames, not display labels",
      );
      const file = args[3];
      const name = basename(file);
      stored.set(name, await readFile(file));
      if (fail) throw new Error("response lost");
      return { stdout: "" };
    }
    throw new Error("unexpected command");
  };
  const assets = new ReleaseAssets({
    run,
    root,
    directory: join(root, "downloads"),
    wait: async () => {},
  });
  return {
    assets,
    files,
    stored,
    uploads: () => uploads,
    loseResponse: () => {
      fail = true;
    },
  };
}
test("retains each asset once and recovers lost upload responses by verifying bytes", async (t) => {
  const f = await fixture(t);
  f.loseResponse();
  await f.assets.retain(f.files, {
    hostTag: "v0.100.0",
    record: { sdk_version: "0.1.0" },
  });
  assert.equal(f.uploads(), 3);
  await f.assets.retain(f.files, {
    hostTag: "v0.100.0",
    record: { sdk_version: "0.1.0" },
  });
  assert.equal(f.uploads(), 3);
});
test("conflicting retained assets fail before upload without clobber", async (t) => {
  const f = await fixture(t);
  f.stored.set("plugin-sdk-0.1.0.tgz", Buffer.from("other"));
  await assert.rejects(
    f.assets.retain(f.files, {
      hostTag: "v0.100.0",
      record: { sdk_version: "0.1.0" },
    }),
    /conflict/,
  );
  assert.equal(f.uploads(), 0);
});
