#!/usr/bin/env node

import { createHash } from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { gunzipSync, gzipSync } from "node:zlib";

export const REMOTE_HELPERS = [
  {
    name: "agentctl-linux-amd64",
    platform: "linux/amd64",
    asset: "agentctl-linux-amd64.gz",
  },
  {
    name: "agentctl-linux-arm64",
    platform: "linux/arm64",
    asset: "agentctl-linux-arm64.gz",
  },
  {
    name: "agentctl-darwin-amd64",
    platform: "darwin/amd64",
    asset: "agentctl-darwin-amd64.gz",
  },
  {
    name: "agentctl-darwin-arm64",
    platform: "darwin/arm64",
    asset: "agentctl-darwin-arm64.gz",
  },
];

const MANIFEST_NAME = "remote-helpers.json";
const IDENTITY_NAME = "identity.json";
const MAX_HELPER_BYTES = 256 * 1024 * 1024;
const VERSION_PATTERN = /^v?[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$/;
const COMMIT_PATTERN = /^(?:[a-f0-9]{40}|[a-f0-9]{64})$/;
const HELPER_ASSET_PATTERN =
  /^agentctl-(?:linux|darwin)-(?:amd64|arm64)\.gz(?:\.sha256)?$/;

function fail(message) {
  throw new Error(message);
}

function assertExactKeys(value, keys, context) {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    fail(`${context} must be an object`);
  }
  const actual = Object.keys(value).sort();
  const expected = [...keys].sort();
  if (
    actual.length !== expected.length ||
    actual.some((key, index) => key !== expected[index])
  ) {
    fail(`${context} has missing or unexpected fields`);
  }
}

function readJson(file, context) {
  let data;
  try {
    data = fs.readFileSync(file, "utf8");
  } catch (error) {
    fail(`${context} could not be read: ${error.message}`);
  }
  try {
    return JSON.parse(data);
  } catch (error) {
    fail(`${context} is not valid JSON: ${error.message}`);
  }
}

function sha256(data) {
  return createHash("sha256").update(data).digest("hex");
}

function listRegularFiles(directory, context) {
  let entries;
  try {
    entries = fs.readdirSync(directory, { withFileTypes: true });
  } catch (error) {
    fail(`${context} could not be read: ${error.message}`);
  }
  const names = [];
  for (const entry of entries) {
    if (!entry.isFile())
      fail(`${context} contains unexpected entry ${entry.name}`);
    names.push(entry.name);
  }
  return names.sort();
}

function validateIdentity(version, commit) {
  if (typeof version !== "string" || !VERSION_PATTERN.test(version)) {
    fail(`release version ${JSON.stringify(version)} is invalid`);
  }
  if (typeof commit !== "string" || !COMMIT_PATTERN.test(commit)) {
    fail("release commit must be a full lowercase Git SHA");
  }
}

function helperInputRecords(binDir) {
  const entries = listRegularFiles(binDir, "canonical helper input");
  const expected = REMOTE_HELPERS.map(({ name }) => name).sort();
  for (const name of expected) {
    if (!entries.includes(name)) fail(`missing remote helper ${name}`);
  }
  for (const name of entries) {
    if (!expected.includes(name)) fail(`unexpected canonical helper ${name}`);
  }

  return REMOTE_HELPERS.map((helper) => {
    const source = path.join(binDir, helper.name);
    const info = fs.lstatSync(source);
    if (!info.isFile() || info.isSymbolicLink())
      fail(`canonical helper ${helper.name} is not a regular file`);
    if ((info.mode & 0o111) === 0)
      fail(`canonical helper ${helper.name} is not executable`);
    if (info.size <= 0 || info.size > MAX_HELPER_BYTES)
      fail(`canonical helper ${helper.name} has an invalid size`);
    const bytes = fs.readFileSync(source);
    return {
      ...helper,
      source,
      bytes,
      digest: sha256(bytes),
      sizeBytes: bytes.length,
    };
  });
}

function manifestFor(version, commit, variant, helpers) {
  return {
    schema_version: 1,
    version,
    commit,
    variant,
    helpers: helpers.map(({ platform, asset, digest, sizeBytes }) => ({
      platform,
      asset,
      sha256: digest,
      size_bytes: sizeBytes,
    })),
  };
}

function writeJson(file, value) {
  fs.writeFileSync(file, `${JSON.stringify(value, null, 2)}\n`, {
    mode: 0o644,
    flag: "wx",
  });
}

/** Build raw helpers and deterministic Stable release assets into a new directory. */
export function buildRemoteHelperArtifact({
  binDir,
  outputDir,
  version,
  commit,
  stable,
}) {
  validateIdentity(version, commit);
  if (typeof stable !== "boolean") fail("stable must be a boolean");
  if (!binDir || !outputDir) fail("binDir and outputDir are required");
  if (fs.existsSync(outputDir))
    fail(`output directory already exists: ${outputDir}`);

  const helpers = helperInputRecords(binDir);
  fs.mkdirSync(outputDir, { recursive: true });
  const outputBin = path.join(outputDir, "bin");
  fs.mkdirSync(outputBin);
  for (const helper of helpers) {
    const target = path.join(outputBin, helper.name);
    fs.copyFileSync(helper.source, target, fs.constants.COPYFILE_EXCL);
    fs.chmodSync(target, 0o755);
  }
  writeJson(path.join(outputDir, IDENTITY_NAME), { version, commit });
  if (!stable) return;

  const assetsDir = path.join(outputDir, "assets");
  const manifestsDir = path.join(outputDir, "manifests");
  fs.mkdirSync(assetsDir);
  fs.mkdirSync(manifestsDir);
  for (const helper of helpers) {
    const compressed = gzipSync(helper.bytes, { level: 9, mtime: 0 });
    fs.writeFileSync(path.join(assetsDir, helper.asset), compressed, {
      mode: 0o644,
      flag: "wx",
    });
    fs.writeFileSync(
      path.join(assetsDir, `${helper.asset}.sha256`),
      `${sha256(compressed)}  ${helper.asset}\n`,
      { mode: 0o644, flag: "wx" },
    );
  }
  for (const variant of ["standard", "full"]) {
    const variantDir = path.join(manifestsDir, variant);
    fs.mkdirSync(variantDir);
    writeJson(
      path.join(variantDir, MANIFEST_NAME),
      manifestFor(version, commit, variant, helpers),
    );
  }
}

function parseManifest(manifest, expectedVariant, expectedIdentity) {
  assertExactKeys(
    manifest,
    ["schema_version", "version", "commit", "variant", "helpers"],
    "remote helper manifest",
  );
  if (manifest.schema_version !== 1)
    fail(
      `unsupported remote helper manifest schema version ${manifest.schema_version}`,
    );
  validateIdentity(manifest.version, manifest.commit);
  if (
    expectedIdentity &&
    (manifest.version !== expectedIdentity.version ||
      manifest.commit !== expectedIdentity.commit)
  ) {
    fail("remote helper manifest identity does not match the expected release");
  }
  if (manifest.variant !== expectedVariant)
    fail(`remote helper manifest variant must be ${expectedVariant}`);
  if (
    !Array.isArray(manifest.helpers) ||
    manifest.helpers.length !== REMOTE_HELPERS.length
  ) {
    fail(
      `remote helper manifest must contain exactly ${REMOTE_HELPERS.length} platform records`,
    );
  }
  const records = new Map();
  for (const record of manifest.helpers) {
    assertExactKeys(
      record,
      ["platform", "asset", "sha256", "size_bytes"],
      "remote helper record",
    );
    const helper = REMOTE_HELPERS.find(
      (candidate) => candidate.platform === record.platform,
    );
    if (!helper)
      fail(
        `unsupported remote helper platform ${JSON.stringify(record.platform)}`,
      );
    if (records.has(record.platform))
      fail(`duplicate remote helper platform ${record.platform}`);
    if (
      record.asset !== helper.asset ||
      path.basename(record.asset) !== record.asset ||
      record.asset.includes("..")
    ) {
      fail(`invalid remote helper asset for ${record.platform}`);
    }
    if (
      typeof record.sha256 !== "string" ||
      !/^[a-f0-9]{64}$/.test(record.sha256)
    ) {
      fail(`invalid remote helper digest for ${record.platform}`);
    }
    if (
      !Number.isSafeInteger(record.size_bytes) ||
      record.size_bytes <= 0 ||
      record.size_bytes > MAX_HELPER_BYTES
    ) {
      fail(`invalid remote helper size for ${record.platform}`);
    }
    records.set(record.platform, record);
  }
  if (records.size !== REMOTE_HELPERS.length)
    fail("remote helper manifest omits a supported platform");
  return records;
}

function verifyCompressedAssets(assetsDir, helpers) {
  const expectedNames = helpers
    .flatMap(({ asset }) => [asset, `${asset}.sha256`])
    .sort();
  const actualNames = listRegularFiles(assetsDir, "remote helper assets");
  if (
    actualNames.length !== expectedNames.length ||
    actualNames.some((name, index) => name !== expectedNames[index])
  ) {
    fail("remote helper assets are missing or contain unexpected files");
  }
  for (const helper of helpers) {
    const compressed = fs.readFileSync(path.join(assetsDir, helper.asset));
    const sidecar = fs.readFileSync(
      path.join(assetsDir, `${helper.asset}.sha256`),
      "utf8",
    );
    if (sidecar !== `${sha256(compressed)}  ${helper.asset}\n`)
      fail(`checksum sidecar does not match ${helper.asset}`);
    let uncompressed;
    try {
      uncompressed = gunzipSync(compressed, {
        maxOutputLength: MAX_HELPER_BYTES,
      });
    } catch (error) {
      fail(`compressed helper ${helper.asset} is invalid: ${error.message}`);
    }
    if (
      uncompressed.length !== helper.sizeBytes ||
      sha256(uncompressed) !== helper.digest
    ) {
      fail(
        `compressed helper ${helper.asset} does not match its canonical bytes`,
      );
    }
  }
}

/** Verify a downloaded canonical helper artifact against its identity and payloads. */
export function verifyRemoteHelperArtifact({
  artifactDir,
  version,
  commit,
  stable,
}) {
  validateIdentity(version, commit);
  if (typeof stable !== "boolean") fail("stable must be a boolean");
  const expectedEntries = stable
    ? ["assets", "bin", "identity.json", "manifests"]
    : ["bin", "identity.json"];
  const actualEntries = fs.readdirSync(artifactDir).sort();
  if (
    actualEntries.length !== expectedEntries.length ||
    actualEntries.some((entry, index) => entry !== expectedEntries[index])
  ) {
    fail("canonical helper artifact has missing or unexpected entries");
  }
  const identity = readJson(
    path.join(artifactDir, IDENTITY_NAME),
    "canonical helper identity",
  );
  assertExactKeys(identity, ["version", "commit"], "canonical helper identity");
  if (identity.version !== version || identity.commit !== commit)
    fail("canonical helper identity does not match the expected release");
  const helpers = helperInputRecords(path.join(artifactDir, "bin"));
  if (!stable) return true;

  verifyCompressedAssets(path.join(artifactDir, "assets"), helpers);
  const manifestEntries = fs
    .readdirSync(path.join(artifactDir, "manifests"))
    .sort();
  if (manifestEntries.join(",") !== "full,standard")
    fail("canonical helper artifact has missing or unexpected manifests");
  for (const variant of ["standard", "full"]) {
    const manifest = readJson(
      path.join(artifactDir, "manifests", variant, MANIFEST_NAME),
      `${variant} manifest`,
    );
    const records = parseManifest(manifest, variant, { version, commit });
    for (const helper of helpers) {
      const record = records.get(helper.platform);
      if (
        record.sha256 !== helper.digest ||
        record.size_bytes !== helper.sizeBytes
      ) {
        fail(
          `${variant} manifest does not match canonical helper ${helper.name}`,
        );
      }
    }
  }
  return true;
}

function runtimeBinaryNames(binDir) {
  const actual = listRegularFiles(binDir, "runtime bin directory");
  const launcher = actual.includes("kandev.exe") ? "kandev.exe" : "kandev";
  const nativeAgentctl = actual.includes("agentctl.exe")
    ? "agentctl.exe"
    : "agentctl";
  return { actual, launcher, nativeAgentctl };
}

/** Validate standard/full bundle contents; full also accepts legacy no-manifest layouts. */
export function verifyRemoteHelperBundle({ bundleDir, variant }) {
  if (variant !== "standard" && variant !== "full")
    fail(`invalid runtime bundle variant ${JSON.stringify(variant)}`);
  const binDir = path.join(bundleDir, "bin");
  const { actual, launcher, nativeAgentctl } = runtimeBinaryNames(binDir);
  for (const name of [launcher, nativeAgentctl]) {
    if (!actual.includes(name)) fail(`runtime bundle is missing ${name}`);
  }
  const manifestPath = path.join(bundleDir, MANIFEST_NAME);
  const hasManifest = fs.existsSync(manifestPath);
  if (variant === "standard" && !hasManifest)
    fail("standard runtime manifest is required");

  const helperNames = new Set(REMOTE_HELPERS.map(({ name }) => name));
  const bundledHelpers = actual.filter((name) => helperNames.has(name));
  if (variant === "standard" && bundledHelpers.length > 0) {
    fail(`standard runtime unexpectedly contains ${bundledHelpers[0]}`);
  }
  if (variant === "full") {
    for (const helper of REMOTE_HELPERS) {
      if (!actual.includes(helper.name))
        fail(`full runtime is missing remote helper ${helper.name}`);
    }
  }
  const expected =
    variant === "standard"
      ? [launcher, nativeAgentctl]
      : [launcher, nativeAgentctl, ...REMOTE_HELPERS.map(({ name }) => name)];
  expected.sort();
  const unexpected = actual.find((name) => !expected.includes(name));
  if (unexpected)
    fail(`Unexpected runtime artifact ${unexpected} in runtime bin directory`);
  const missing = expected.find((name) => !actual.includes(name));
  if (missing)
    fail(`Missing runtime artifact ${missing} in runtime bin directory`);

  if (!hasManifest) return true;
  const manifest = readJson(manifestPath, "remote helper manifest");
  const records = parseManifest(manifest, variant);
  if (variant === "full") {
    for (const helper of REMOTE_HELPERS) {
      const record = records.get(helper.platform);
      const bytes = fs.readFileSync(path.join(binDir, helper.name));
      if (
        bytes.length !== record.size_bytes ||
        sha256(bytes) !== record.sha256
      ) {
        fail(`bundled helper ${helper.name} does not match the manifest`);
      }
    }
  }
  return true;
}

/** Refuse to replace a same-named GitHub asset with different bytes. */
export function verifyExistingHelperAssets({ releaseAssets, assetsDir }) {
  if (!Array.isArray(releaseAssets))
    fail("existing release assets must be an array");
  const candidates = listRegularFiles(
    assetsDir,
    "candidate remote helper assets",
  );
  if (
    candidates.length !== REMOTE_HELPERS.length * 2 ||
    candidates.some((name) => !HELPER_ASSET_PATTERN.test(name))
  ) {
    fail("candidate remote helper assets are missing or unexpected");
  }
  const existing = new Map();
  for (const asset of releaseAssets) {
    if (!asset || typeof asset.name !== "string")
      fail("existing release asset has no name");
    if (!HELPER_ASSET_PATTERN.test(asset.name)) continue;
    if (existing.has(asset.name))
      fail(`existing release contains duplicate asset ${asset.name}`);
    existing.set(asset.name, asset.digest);
  }
  for (const name of candidates) {
    if (!existing.has(name)) continue;
    const digest = existing.get(name);
    if (typeof digest !== "string" || !/^sha256:[a-f0-9]{64}$/.test(digest)) {
      fail(`existing release asset ${name} has no SHA-256 digest`);
    }
    const localDigest = `sha256:${sha256(fs.readFileSync(path.join(assetsDir, name)))}`;
    if (digest !== localDigest)
      fail(
        `existing release asset ${name} already exists with different bytes`,
      );
  }
  return true;
}

function argsToMap(args) {
  const result = new Map();
  for (let index = 0; index < args.length; index += 2) {
    const key = args[index];
    if (!key?.startsWith("--") || index + 1 >= args.length || result.has(key))
      fail(`invalid command arguments near ${key}`);
    result.set(key.slice(2), args[index + 1]);
  }
  return result;
}

function requireArg(values, key) {
  const value = values.get(key);
  if (!value) fail(`--${key} is required`);
  return value;
}

function parseBoolean(value, key) {
  if (value === "true") return true;
  if (value === "false") return false;
  fail(`--${key} must be true or false`);
}

function runCli(argv) {
  const [command, ...args] = argv;
  const values = argsToMap(args);
  switch (command) {
    case "build":
      buildRemoteHelperArtifact({
        binDir: requireArg(values, "bin-dir"),
        outputDir: requireArg(values, "output-dir"),
        version: requireArg(values, "version"),
        commit: requireArg(values, "commit"),
        stable: parseBoolean(requireArg(values, "stable"), "stable"),
      });
      return;
    case "verify-artifact":
      verifyRemoteHelperArtifact({
        artifactDir: requireArg(values, "artifact-dir"),
        version: requireArg(values, "version"),
        commit: requireArg(values, "commit"),
        stable: parseBoolean(requireArg(values, "stable"), "stable"),
      });
      return;
    case "verify-bundle":
      verifyRemoteHelperBundle({
        bundleDir: requireArg(values, "bundle-dir"),
        variant: requireArg(values, "variant"),
      });
      return;
    case "verify-existing-release": {
      const release = readJson(
        requireArg(values, "release-json"),
        "existing GitHub release",
      );
      if (!release || !Array.isArray(release.assets))
        fail("existing GitHub release has no asset list");
      verifyExistingHelperAssets({
        releaseAssets: release.assets,
        assetsDir: requireArg(values, "assets-dir"),
      });
      return;
    }
    default:
      fail(
        "usage: remote-helper-assets.mjs <build|verify-artifact|verify-bundle|verify-existing-release> [options]",
      );
  }
}

if (
  process.argv[1] &&
  path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)
) {
  try {
    runCli(process.argv.slice(2));
  } catch (error) {
    process.stderr.write(`${error.message}\n`);
    process.exitCode = 1;
  }
}
