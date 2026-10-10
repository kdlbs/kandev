import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import {
  buildRemoteHelperArtifact,
  verifyExistingHelperAssets,
  verifyRemoteHelperArtifact,
  verifyRemoteHelperBundle,
} from "./remote-helper-assets.mjs";

const VERSION = "v1.2.3";
const COMMIT = "a".repeat(40);
const HELPERS = [
  "agentctl-linux-amd64",
  "agentctl-linux-arm64",
  "agentctl-darwin-amd64",
  "agentctl-darwin-arm64",
];

function tempRoot(t) {
  const root = fs.mkdtempSync(
    path.join(os.tmpdir(), "kandev-remote-helper-assets-"),
  );
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  return root;
}

function writeHelpers(root, name = "input-bin") {
  const binDir = path.join(root, name);
  fs.mkdirSync(binDir);
  for (const [index, helper] of HELPERS.entries()) {
    fs.writeFileSync(path.join(binDir, helper), `helper ${index}\n`, {
      mode: 0o755,
    });
  }
  return binDir;
}

function sha256(data) {
  return createHash("sha256").update(data).digest("hex");
}

test("builds canonical compressed assets, checksums, and both manifest variants", (t) => {
  const root = tempRoot(t);
  const binDir = writeHelpers(root);
  const outputDir = path.join(root, "artifact");

  buildRemoteHelperArtifact({
    binDir,
    outputDir,
    version: VERSION,
    commit: COMMIT,
    stable: true,
  });
  verifyRemoteHelperArtifact({
    artifactDir: outputDir,
    version: VERSION,
    commit: COMMIT,
    stable: true,
  });

  const standard = JSON.parse(
    fs.readFileSync(
      path.join(outputDir, "manifests/standard/remote-helpers.json"),
      "utf8",
    ),
  );
  const full = JSON.parse(
    fs.readFileSync(
      path.join(outputDir, "manifests/full/remote-helpers.json"),
      "utf8",
    ),
  );
  assert.equal(standard.variant, "standard");
  assert.equal(full.variant, "full");
  assert.equal(standard.version, VERSION);
  assert.equal(standard.commit, COMMIT);
  assert.equal(standard.helpers.length, HELPERS.length);
  assert.deepEqual(
    standard.helpers.map(({ platform, asset }) => [platform, asset]),
    [
      ["linux/amd64", "agentctl-linux-amd64.gz"],
      ["linux/arm64", "agentctl-linux-arm64.gz"],
      ["darwin/amd64", "agentctl-darwin-amd64.gz"],
      ["darwin/arm64", "agentctl-darwin-arm64.gz"],
    ],
  );
  assert.deepEqual(standard.helpers, full.helpers);
});

test("helper asset compression is byte-for-byte reproducible", (t) => {
  const root = tempRoot(t);
  const binDir = writeHelpers(root);
  const first = path.join(root, "first");
  const second = path.join(root, "second");

  buildRemoteHelperArtifact({
    binDir,
    outputDir: first,
    version: VERSION,
    commit: COMMIT,
    stable: true,
  });
  buildRemoteHelperArtifact({
    binDir,
    outputDir: second,
    version: VERSION,
    commit: COMMIT,
    stable: true,
  });

  for (const helper of HELPERS) {
    const asset = `${helper}.gz`;
    assert.deepEqual(
      fs.readFileSync(path.join(first, "assets", asset)),
      fs.readFileSync(path.join(second, "assets", asset)),
    );
    assert.deepEqual(
      fs.readFileSync(path.join(first, "assets", `${asset}.sha256`)),
      fs.readFileSync(path.join(second, "assets", `${asset}.sha256`)),
    );
  }
});

test("rejects missing manifests and duplicate or mismatched canonical helper records", (t) => {
  for (const mutation of [
    "missing manifest",
    "duplicate platform",
    "wrong digest",
  ]) {
    const root = tempRoot(t);
    const binDir = writeHelpers(root);
    const artifactDir = path.join(root, "artifact");
    buildRemoteHelperArtifact({
      binDir,
      outputDir: artifactDir,
      version: VERSION,
      commit: COMMIT,
      stable: true,
    });
    const manifestPath = path.join(
      artifactDir,
      "manifests/standard/remote-helpers.json",
    );
    if (mutation === "missing manifest") {
      fs.rmSync(manifestPath);
    } else {
      const manifest = JSON.parse(fs.readFileSync(manifestPath, "utf8"));
      if (mutation === "duplicate platform") {
        manifest.helpers[1].platform = manifest.helpers[0].platform;
      } else {
        manifest.helpers[0].sha256 = "0".repeat(64);
      }
      fs.writeFileSync(manifestPath, JSON.stringify(manifest));
    }
    assert.throws(
      () =>
        verifyRemoteHelperArtifact({
          artifactDir,
          version: VERSION,
          commit: COMMIT,
          stable: true,
        }),
      mutation === "missing manifest"
        ? /could not be read/
        : mutation === "duplicate platform"
          ? /duplicate/
          : /does not match/,
    );
  }
});

test("nightly helper artifacts remain complete and omit fetch assets and manifests", (t) => {
  const root = tempRoot(t);
  const binDir = writeHelpers(root);
  const outputDir = path.join(root, "nightly");

  buildRemoteHelperArtifact({
    binDir,
    outputDir,
    version: "v1.3.0-nightly.sha123456789abc",
    commit: COMMIT,
    stable: false,
  });
  verifyRemoteHelperArtifact({
    artifactDir: outputDir,
    version: "v1.3.0-nightly.sha123456789abc",
    commit: COMMIT,
    stable: false,
  });

  assert.deepEqual(fs.readdirSync(outputDir).sort(), ["bin", "identity.json"]);
});

test("rejects incomplete, unexpected, and non-executable canonical helper inputs", (t) => {
  const root = tempRoot(t);
  const missing = writeHelpers(root);
  fs.rmSync(path.join(missing, HELPERS[0]));
  assert.throws(
    () =>
      buildRemoteHelperArtifact({
        binDir: missing,
        outputDir: path.join(root, "missing"),
        version: VERSION,
        commit: COMMIT,
        stable: true,
      }),
    /missing remote helper agentctl-linux-amd64/,
  );

  const unexpected = writeHelpers(root, "unexpected-bin");
  fs.writeFileSync(
    path.join(unexpected, "agentctl-windows-amd64"),
    "unsupported",
    { mode: 0o755 },
  );
  assert.throws(
    () =>
      buildRemoteHelperArtifact({
        binDir: unexpected,
        outputDir: path.join(root, "unexpected"),
        version: VERSION,
        commit: COMMIT,
        stable: true,
      }),
    /unexpected canonical helper/,
  );

  const nonExecutable = writeHelpers(root, "non-executable-bin");
  fs.chmodSync(path.join(nonExecutable, HELPERS[0]), 0o644);
  assert.throws(
    () =>
      buildRemoteHelperArtifact({
        binDir: nonExecutable,
        outputDir: path.join(root, "non-executable"),
        version: VERSION,
        commit: COMMIT,
        stable: true,
      }),
    /not executable/,
  );
});

test("rejects mismatched helper archives, manifest records, and bundle variants", (t) => {
  const root = tempRoot(t);
  const binDir = writeHelpers(root);
  const artifactDir = path.join(root, "artifact");
  buildRemoteHelperArtifact({
    binDir,
    outputDir: artifactDir,
    version: VERSION,
    commit: COMMIT,
    stable: true,
  });

  const gzipPath = path.join(artifactDir, "assets/agentctl-linux-amd64.gz");
  fs.appendFileSync(gzipPath, "corruption");
  assert.throws(
    () =>
      verifyRemoteHelperArtifact({
        artifactDir,
        version: VERSION,
        commit: COMMIT,
        stable: true,
      }),
    /checksum|content|gzip/i,
  );

  const bundle = path.join(root, "standard");
  fs.mkdirSync(path.join(bundle, "bin"), { recursive: true });
  fs.writeFileSync(path.join(bundle, "bin/kandev"), "launcher", {
    mode: 0o755,
  });
  fs.writeFileSync(path.join(bundle, "bin/agentctl"), "native", {
    mode: 0o755,
  });
  fs.copyFileSync(
    path.join(root, "artifact/manifests/standard/remote-helpers.json"),
    path.join(bundle, "remote-helpers.json"),
  );
  assert.equal(
    verifyRemoteHelperBundle({ bundleDir: bundle, variant: "standard" }),
    true,
  );
  fs.writeFileSync(path.join(bundle, "bin/agentctl-linux-amd64"), "forbidden", {
    mode: 0o755,
  });
  assert.throws(
    () => verifyRemoteHelperBundle({ bundleDir: bundle, variant: "standard" }),
    /unexpectedly contains/,
  );
});

test("accepts a legacy complete bundle while refusing a standard bundle without its manifest", (t) => {
  const root = tempRoot(t);
  const legacy = path.join(root, "legacy");
  const binDir = path.join(legacy, "bin");
  fs.mkdirSync(binDir, { recursive: true });
  for (const name of ["kandev", "agentctl", ...HELPERS]) {
    fs.writeFileSync(path.join(binDir, name), name, { mode: 0o755 });
  }

  assert.equal(
    verifyRemoteHelperBundle({ bundleDir: legacy, variant: "full" }),
    true,
  );
  assert.throws(
    () => verifyRemoteHelperBundle({ bundleDir: legacy, variant: "standard" }),
    /manifest is required/,
  );
});

test("refuses to overwrite an existing helper release asset with different bytes", (t) => {
  const root = tempRoot(t);
  const binDir = writeHelpers(root);
  const artifactDir = path.join(root, "artifact");
  buildRemoteHelperArtifact({
    binDir,
    outputDir: artifactDir,
    version: VERSION,
    commit: COMMIT,
    stable: true,
  });
  const assetPath = path.join(artifactDir, "assets/agentctl-linux-amd64.gz");
  const digest = sha256(fs.readFileSync(assetPath));

  assert.equal(
    verifyExistingHelperAssets({
      releaseAssets: [
        { name: "agentctl-linux-amd64.gz", digest: `sha256:${digest}` },
      ],
      assetsDir: path.join(artifactDir, "assets"),
    }),
    true,
  );
  assert.throws(
    () =>
      verifyExistingHelperAssets({
        releaseAssets: [
          {
            name: "agentctl-linux-amd64.gz",
            digest: `sha256:${"0".repeat(64)}`,
          },
        ],
        assetsDir: path.join(artifactDir, "assets"),
      }),
    /already exists with different bytes/,
  );
  assert.throws(
    () =>
      verifyExistingHelperAssets({
        releaseAssets: [{ name: "agentctl-linux-amd64.gz", digest: null }],
        assetsDir: path.join(artifactDir, "assets"),
      }),
    /has no SHA-256 digest/,
  );
});
