import assert from "node:assert/strict";
import {
  mkdtemp,
  mkdir,
  writeFile,
  readFile,
  copyFile,
  rm,
} from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { packCandidate, packConfig } from "../pluginsdk/pack-candidate.mjs";
import { CandidateStore } from "./sdk-candidate-store.mjs";
import { digest } from "./sdk-artifact.mjs";

async function fixture(t) {
  const root = await mkdtemp(join(tmpdir(), "sdk-retained-candidate-"));
  t.after(() => rm(root, { recursive: true, force: true }));
  const pkg = join(root, "package");
  await mkdir(join(pkg, "src"), { recursive: true });
  const record = {
    schema_version: 1,
    sdk_version: "0.1.0",
    host_release: "0.100.0",
    min_kandev_version: "0.100.0",
    api_versions: [1, 2],
    change_kind: "initial",
  };
  const raw = JSON.stringify(record, null, 2) + "\n";
  const files = {
    "package.json": JSON.stringify({
      name: "@kdlbs/plugin-sdk",
      version: "0.1.0",
      files: ["src", "sdk-release.json", "README.md", "LICENSE"],
    }),
    "sdk-release.json": raw,
    "src/index.ts": "export type Value = string;\n",
    "README.md": "Fixture\n",
    LICENSE: "Fixture license\n",
  };
  for (const [name, data] of Object.entries(files))
    await writeFile(join(pkg, name), data);
  const packed = await packCandidate({
    directory: pkg,
    output: join(root, "build"),
  });
  const source = {
    commit: "a".repeat(40),
    hostTag: "v0.100.0",
    sdkTag: "pluginsdk/v0.1.0",
    record,
    recordDigest: digest(raw),
    packConfig,
  };
  const workflow = {
    workflow_ref: "kdlbs/kandev/.github/workflows/release.yml@refs/heads/main",
    workflow_sha: "b".repeat(40),
    run_id: "123",
    run_attempt: "1",
  };
  const retained = {};
  let builds = 0;
  const assets = { download: async () => ({ ...retained }) };
  const store = new CandidateStore({
    directory: join(root, "candidate"),
    assets,
    workflow,
    build: async () => {
      builds++;
      return packed;
    },
    sign: async (receipt, signature) =>
      writeFile(signature, "signed:" + digest(await readFile(receipt))),
    verify: async (receipt, signature) => {
      assert.equal(
        await readFile(signature, "utf8"),
        "signed:" + digest(await readFile(receipt)),
      );
      return "C".repeat(40);
    },
    verifyWorkflow: async () => {},
  });
  const retain = async (candidate) => {
    for (const key of ["archive", "receiptPath", "signaturePath"]) {
      const path = join(root, "retained-" + key);
      await copyFile(candidate[key], path);
      retained[key] = path;
    }
  };
  return {
    root,
    store,
    source,
    packed,
    retained,
    retain,
    builds: () => builds,
  };
}
// @covers AC-PLUGINS-SDK-DISTRIBUTION-003.2
test("reuses retained bytes without repacking a later host checkout", async (t) => {
  const f = await fixture(t);
  const first = await f.store.obtain(f.source, {
    allowCreate: true,
    bootstrapAllowed: true,
  });
  assert.equal(first.integrity, f.packed.integrity);
  await f.retain(first);
  f.store.build = async () => {
    throw new Error("must not repack reused SDK");
  };
  const reused = await f.store.obtain(f.source, { allowCreate: false });
  assert.equal(reused.integrity, first.integrity);
  assert.equal(reused.receipt.run_id, "123");
});
test("rebuilds a lost archive only when original tools reproduce its authenticated integrity", async (t) => {
  const f = await fixture(t);
  const first = await f.store.obtain(f.source, {
    allowCreate: true,
    bootstrapAllowed: true,
  });
  await f.retain(first);
  delete f.retained.archive;
  assert.equal(
    (await f.store.obtain(f.source, { allowCreate: false })).integrity,
    first.integrity,
  );
  assert.equal(f.builds(), 2);
  const changed = join(f.root, "changed.tgz");
  await writeFile(changed, "different bytes");
  f.store.build = async () => ({
    ...f.packed,
    archive: changed,
    integrity: "sha512-changed",
  });
  await assert.rejects(
    f.store.obtain(f.source, { allowCreate: false }),
    /integrity|rebuild/,
  );
});
test("missing receipt after publication and conflicting existing assets stop recovery", async (t) => {
  const f = await fixture(t);
  await assert.rejects(
    f.store.obtain(f.source, { allowCreate: false }),
    /receipt/,
  );
  assert.equal(f.builds(), 0);
  const conflict = join(f.root, "conflicting.tgz");
  await writeFile(conflict, "other archive");
  f.retained.archive = conflict;
  await assert.rejects(
    f.store.obtain(f.source, { allowCreate: true, bootstrapAllowed: true }),
    /conflict|integrity/,
  );
});
test("interrupted initial retention can finish a missing signature without replacing the receipt", async (t) => {
  const f = await fixture(t);
  const first = await f.store.obtain(f.source, {
    allowCreate: true,
    bootstrapAllowed: true,
  });
  await f.retain(first);
  delete f.retained.signaturePath;
  f.store.workflow = { ...f.store.workflow, run_id: "456" };
  const recovered = await f.store.obtain(f.source, {
    allowCreate: true,
    bootstrapAllowed: true,
  });
  assert.equal(recovered.receipt.run_id, "123");
  assert.equal(f.builds(), 2);
  await assert.rejects(
    f.store.obtain(f.source, { allowCreate: false }),
    /signature|receipt/,
  );
});
