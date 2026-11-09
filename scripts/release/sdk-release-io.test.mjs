import test from "node:test";
import assert from "node:assert/strict";
import { ReleaseIO, projectVerifiedRegistryState } from "./sdk-release-io.mjs";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { cp, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { digest } from "./sdk-artifact.mjs";

test("release build preserves the packed record identity through real consumer verification", async (t) => {
  const root = await mkdtemp(join(tmpdir(), "sdk-release-build-"));
  t.after(() => rm(root, { recursive: true, force: true }));
  const run = promisify(execFile);
  const repository = {
    git: (...args) => run("git", args, { cwd: root }),
  };
  for (const path of [
    "pluginsdk",
    "apps/packages/plugin-sdk",
    "scripts/pluginsdk/pack-config.json",
  ]) {
    await cp(
      fileURLToPath(new URL(`../../${path}`, import.meta.url)),
      join(root, path),
      {
        recursive: true,
        filter: (path) =>
          !path
            .split("/")
            .some((part) => ["node_modules", ".tools"].includes(part)),
      },
    );
  }
  const record = JSON.parse(
    await readFile(join(root, "pluginsdk/sdk-release.json"), "utf8"),
  );
  record.host_release = "0.100.0";
  record.min_kandev_version = "0.100.0";
  const bytes = JSON.stringify(record, null, 2) + "\n";
  for (const path of ["pluginsdk", "apps/packages/plugin-sdk"])
    await writeFile(join(root, path, "sdk-release.json"), bytes);
  await repository.git("init", "--quiet");
  await repository.git("add", ".");
  await repository.git(
    "-c",
    "user.name=SDK test",
    "-c",
    "user.email=sdk@example.test",
    "commit",
    "--quiet",
    "-m",
    "test fixture",
  );
  const source = {
    commit: (await repository.git("rev-parse", "HEAD")).stdout.trim(),
    record,
    recordDigest: digest(bytes),
    packConfig: JSON.parse(
      await readFile(join(root, "scripts/pluginsdk/pack-config.json"), "utf8"),
    ),
  };
  const io = new ReleaseIO({
    root,
    directory: join(root, "build"),
    repository,
    source,
  });
  const candidate = await io.build(source);
  assert.equal(candidate.recordDigest, source.recordDigest);
  // A different signed record must still fail before any consumer is accepted.
  await assert.rejects(
    io.build({ ...source, recordDigest: "0".repeat(64) }),
    /packed SDK record differs/,
  );
});

test("npm adapter publishes the retained archive without changing latest implicitly", async () => {
  const calls = [];
  const io = Object.create(ReleaseIO.prototype);
  io.npm = async (args) => calls.push(args);
  await io.publish({ archive: "/tmp/original/plugin-sdk-0.1.0.tgz" });
  await io.moveLatest("0.1.0");
  assert.deepEqual(calls, [
    [
      "publish",
      "/tmp/original/plugin-sdk-0.1.0.tgz",
      "--provenance",
      "--access",
      "public",
      "--ignore-scripts",
      "--registry",
      "https://registry.npmjs.org",
      "--tag",
      "sdk-release",
    ],
    [
      "dist-tag",
      "add",
      "@kdlbs/plugin-sdk@0.1.0",
      "latest",
      "--registry",
      "https://registry.npmjs.org",
    ],
  ]);
});

test("public consumer visibility retries are bounded and never report exhausted verification as success", async () => {
  const io = Object.create(ReleaseIO.prototype);
  io.source = {
    record: { sdk_version: "0.1.0" },
    recordDigest: "a".repeat(64),
    commit: "b".repeat(40),
    hostTag: "v0.100.0",
    sdkTag: "pluginsdk/v0.1.0",
  };
  let attempts = 0;
  io.smoke = async () => {
    attempts++;
    throw new Error("proxy not visible");
  };
  io.wait = async () => {};
  await assert.rejects(
    io.verifyConsumers({ integrity: "sha512-fixture" }, "oidc"),
  );
  assert.equal(attempts, 3);
});

test("registry observation clock and sleep are injectable", async () => {
  let now = 100;
  const waits = [];
  const io = new ReleaseIO({
    root: "/tmp/sdk-release-io",
    directory: "/tmp/sdk-release-io/work",
    repository: {},
    source: {},
    now: () => now,
    registryWait: async (milliseconds) => {
      waits.push(milliseconds);
      now += milliseconds;
    },
  });
  assert.equal(io.now(), 100);
  await io.registryWait(30_000);
  assert.equal(io.now(), 30_100);
  assert.deepEqual(waits, [30_000]);
});

test("verified registry projection preserves the observed version", () => {
  const projected = projectVerifiedRegistryState(
    {
      packageExists: true,
      latest: "0.1.0",
      version: {
        version: "0.1.0",
        dist: { integrity: "sha512-example" },
      },
    },
    {
      integrity: "sha512-example",
      recordDigest: "a".repeat(64),
      provenance: true,
    },
  );
  assert.equal(projected.version.version, "0.1.0");
  assert.equal(projected.version.integrity, "sha512-example");
  assert.equal(projected.version.recordDigest, "a".repeat(64));
  assert.equal(projected.version.provenance, true);
  assert.equal(projected.latest, "0.1.0");
});
