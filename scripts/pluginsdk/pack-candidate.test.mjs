import assert from "node:assert/strict";
import { mkdtemp, rm, cp, writeFile, readFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";
import { checkPackedConsumer } from "../../apps/packages/plugin-sdk/test/packed-consumer.node.mjs";
import {
  packCandidate,
  inspectArchive,
  verifyTools,
  validateInventory,
} from "./pack-candidate.mjs";

const config = {
  node: "24.19.0",
  npm: "11.21.0",
  files: [
    "LICENSE",
    "README.md",
    "package.json",
    "sdk-release.json",
    "src/index.ts",
  ],
};
test("requires exact packaging tools before creating an artifact", () => {
  verifyTools(config, "v24.19.0", "11.21.0");
  assert.throws(() => verifyTools(config, "v24.18.0", "11.21.0"), /Node/);
  assert.throws(() => verifyTools(config, "v24.19.0", "11.17.0"), /npm/);
});
test("rejects omitted and unexpected package files", () => {
  validateInventory(config.files, config);
  for (const files of [
    config.files.slice(1),
    [...config.files, "test/leaked.ts"],
    [...config.files, "../secret"],
    [...config.files, "src/extra.ts"],
  ]) {
    assert.throws(() => validateInventory(files, config), /inventory/);
  }
});
// @covers AC-PLUGINS-SDK-DISTRIBUTION-001.2
test("packs one standalone artifact with a verified byte integrity and complete inventory", async () => {
  const output = await mkdtemp(join(tmpdir(), "sdk-pack-"));
  try {
    const candidate = await packCandidate({
      directory: fileURLToPath(
        new URL("../../apps/packages/plugin-sdk", import.meta.url),
      ),
      output,
    });
    assert.deepEqual(candidate.files, config.files);
    assert.match(candidate.integrity, /^sha512-[A-Za-z0-9+/]+={0,2}$/);
    assert.equal(candidate.node, config.node);
    assert.equal(candidate.npm, config.npm);
    const { createHash } = await import("node:crypto");
    const recordBytes = await readFile(
      new URL("../../pluginsdk/sdk-release.json", import.meta.url),
    );
    assert.equal(
      (await inspectArchive(candidate.archive)).recordDigest,
      createHash("sha256").update(recordBytes).digest("hex"),
    );
    await assert.rejects(
      packCandidate({
        directory: fileURLToPath(
          new URL("../../apps/packages/plugin-sdk", import.meta.url),
        ),
        output,
        config: { ...config, npm: "0.0.0" },
      }),
      /npm 0.0.0/,
    );
    const result = await checkPackedConsumer(candidate);
    assert.deepEqual(result.modes, ["NodeNext", "Bundler"]);
  } finally {
    await rm(output, { recursive: true, force: true });
  }
});

test("rejects application imports and dependencies in the actual packed bytes", async () => {
  const root = await mkdtemp(join(tmpdir(), "sdk-invalid-pack-"));
  try {
    const directory = join(root, "package");
    await cp(
      fileURLToPath(new URL("../../apps/packages/plugin-sdk", import.meta.url)),
      directory,
      { recursive: true, filter: (path) => !path.includes("node_modules") },
    );
    await writeFile(
      join(directory, "src/index.ts"),
      'import type { AppState } from "@kandev/web";\n',
    );
    await assert.rejects(
      packCandidate({ directory, output: join(root, "out") }),
      /external import/,
    );
    await writeFile(
      join(directory, "src/index.ts"),
      "export type Value = string;\n",
    );
    const manifestPath = join(directory, "package.json");
    const manifest = JSON.parse(await readFile(manifestPath, "utf8"));
    manifest.dependencies = { react: "19.0.0" };
    await writeFile(manifestPath, JSON.stringify(manifest));
    await assert.rejects(
      packCandidate({ directory, output: join(root, "out") }),
      /no dependencies/,
    );
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});
test("rejects changed archive bytes before installation", async () => {
  const root = await mkdtemp(join(tmpdir(), "sdk-tampered-pack-"));
  try {
    const archive = join(root, "candidate.tgz");
    await writeFile(archive, "tampered");
    await assert.rejects(
      checkPackedConsumer({ archive, integrity: "sha512-original" }),
      /integrity mismatch before/,
    );
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});
