#!/usr/bin/env node

import fs from "node:fs";
import path from "node:path";
import process from "node:process";
import { spawnSync } from "node:child_process";
import { fileURLToPath, pathToFileURL } from "node:url";

const ROOT_DIR = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "../..",
);
const TAURI_CONFIG = path.join(
  ROOT_DIR,
  "apps/desktop/src-tauri/tauri.conf.json",
);
const VERIFIER_MANIFEST = path.join(
  ROOT_DIR,
  "apps/desktop/src-tauri/Cargo.toml",
);

const TARGETS = Object.freeze({
  "darwin-aarch64": {
    platform: "macos-arm64",
    suffix: ".app.tar.gz",
  },
  "darwin-x86_64": {
    platform: "macos-x64",
    suffix: ".app.tar.gz",
  },
  "linux-aarch64": {
    platform: "linux-arm64",
    suffix: ".AppImage.tar.gz",
  },
  "linux-x86_64": {
    platform: "linux-x64",
    suffix: ".AppImage.tar.gz",
  },
  "windows-x86_64": {
    platform: "windows-x64",
    suffix: ".nsis.zip",
  },
});

function fail(message) {
  throw new Error(message);
}

function parseArgs(argv) {
  const [command, ...tokens] = argv;
  if (command !== "generate" && command !== "verify") {
    fail("Usage: updater-manifest.mjs <generate|verify> [options]");
  }

  const options = {};
  for (let index = 0; index < tokens.length; index += 1) {
    const token = tokens[index];
    if (!token.startsWith("--")) {
      fail(`Unexpected argument: ${token}`);
    }
    const key = token.slice(2);
    if (key === "allow-unsigned") {
      options[key] = true;
      continue;
    }
    const value = tokens[index + 1];
    if (!value || value.startsWith("--")) {
      fail(`Missing value for --${key}`);
    }
    options[key] = value;
    index += 1;
  }
  return { command, options };
}

function requireOption(options, key) {
  const value = options[key];
  if (!value) {
    fail(`Missing required option --${key}`);
  }
  return value;
}

function validateReleaseMetadata(version, tag, repository) {
  if (!/^\d+\.\d+\.\d+$/.test(version)) {
    fail(`Updater version must be stable SemVer (X.Y.Z): ${version}`);
  }
  if (tag !== `v${version}`) {
    fail(`Updater tag must be v${version}: ${tag}`);
  }
  if (!/^[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+$/.test(repository)) {
    fail(`Invalid GitHub repository: ${repository}`);
  }
}

function validatePubDate(pubDate) {
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/.exec(
    pubDate,
  );
  if (!match || Number.isNaN(Date.parse(pubDate))) {
    fail(`Updater pub_date must be RFC 3339: ${pubDate}`);
  }
  const [year, month, day, hour, minute, second] = match.slice(1).map(Number);
  const calendar = new Date(Date.UTC(year, month - 1, day));
  if (
    hour > 23 ||
    minute > 59 ||
    second > 59 ||
    calendar.getUTCFullYear() !== year ||
    calendar.getUTCMonth() !== month - 1 ||
    calendar.getUTCDate() !== day
  ) {
    fail(`Updater pub_date must be RFC 3339: ${pubDate}`);
  }
}

function isValidSignature(signature) {
  if (!/^[A-Za-z0-9+/]+={0,2}$/.test(signature) || signature.length % 4 !== 0) {
    return false;
  }
  try {
    return Buffer.from(signature, "base64").length >= 32;
  } catch {
    return false;
  }
}

function releaseUrl(repository, tag, filename) {
  return `https://github.com/${repository}/releases/download/${encodeURIComponent(tag)}/${encodeURIComponent(filename)}`;
}

function listUpdaterArtifacts(assetsDir) {
  const files = fs
    .readdirSync(assetsDir, { withFileTypes: true })
    .filter((entry) => entry.isFile())
    .map((entry) => entry.name);
  const resolved = {};

  for (const [target, descriptor] of Object.entries(TARGETS)) {
    const prefix = `kandev-desktop-${descriptor.platform}-`;
    const matches = files.filter(
      (filename) =>
        filename.startsWith(prefix) && filename.endsWith(descriptor.suffix),
    );
    if (matches.length > 1) {
      fail(
        `Multiple updater artifacts found for ${target}: ${matches.join(", ")}`,
      );
    }
    if (matches.length === 1) {
      resolved[target] = matches[0];
    }
  }

  const signalCount = files.filter((filename) =>
    Object.values(TARGETS).some(({ platform, suffix }) => {
      const prefix = `kandev-desktop-${platform}-`;
      return (
        filename.startsWith(prefix) &&
        (filename.endsWith(suffix) || filename.endsWith(`${suffix}.sig`))
      );
    }),
  ).length;

  return { resolved, signalCount };
}

function requireCompleteArtifacts(assetsDir, allowUnsigned) {
  if (!fs.existsSync(assetsDir) || !fs.statSync(assetsDir).isDirectory()) {
    fail(`Missing updater assets directory: ${assetsDir}`);
  }

  const { resolved, signalCount } = listUpdaterArtifacts(assetsDir);
  const missing = Object.keys(TARGETS).filter((target) => !resolved[target]);
  if (
    missing.length === Object.keys(TARGETS).length &&
    signalCount === 0 &&
    allowUnsigned
  ) {
    return null;
  }
  if (missing.length > 0) {
    fail(
      `Incomplete updater artifact set; missing targets: ${missing.join(", ")}`,
    );
  }

  for (const [target, filename] of Object.entries(resolved)) {
    const signaturePath = path.join(assetsDir, `${filename}.sig`);
    if (!fs.existsSync(signaturePath)) {
      fail(`Missing updater signature for ${target}: ${filename}.sig`);
    }
    const signature = fs.readFileSync(signaturePath, "utf8").trim();
    if (!isValidSignature(signature)) {
      fail(`Invalid updater signature for ${target}: ${filename}.sig`);
    }
  }
  return resolved;
}

function updaterPublicKey(options) {
  if (options["public-key"]) return options["public-key"];
  let config;
  try {
    config = JSON.parse(fs.readFileSync(TAURI_CONFIG, "utf8"));
  } catch (error) {
    fail(`Could not read the embedded updater public key: ${error.message}`);
  }
  const publicKey = config?.plugins?.updater?.pubkey;
  if (typeof publicKey !== "string" || !publicKey) {
    fail("The desktop updater public key is missing");
  }
  return publicKey;
}

function verifyArtifactSignatures(assetsDir, artifacts, publicKey) {
  const pairs = Object.values(artifacts).flatMap((filename) => [
    path.join(assetsDir, filename),
    path.join(assetsDir, `${filename}.sig`),
  ]);
  const result = spawnSync(
    process.env.CARGO || "cargo",
    [
      "run",
      "--quiet",
      "--locked",
      "--manifest-path",
      VERIFIER_MANIFEST,
      "--bin",
      "updater-signature-verifier",
      "--",
      publicKey,
      ...pairs,
    ],
    { encoding: "utf8" },
  );
  if (result.error) {
    fail(
      `Could not run updater signature verification: ${result.error.message}`,
    );
  }
  if (result.status !== 0) {
    fail(
      (
        result.stderr ||
        result.stdout ||
        "Updater signature verification failed"
      ).trim(),
    );
  }
}

function generate(options) {
  const assetsDir = path.resolve(requireOption(options, "assets-dir"));
  const output = path.resolve(requireOption(options, "output"));
  const version = requireOption(options, "version");
  const tag = requireOption(options, "tag");
  const repository = requireOption(options, "repository");
  const notesFile = path.resolve(requireOption(options, "notes-file"));
  const pubDate = requireOption(options, "pub-date");

  validateReleaseMetadata(version, tag, repository);
  validatePubDate(pubDate);
  const artifacts = requireCompleteArtifacts(
    assetsDir,
    options["allow-unsigned"] === true,
  );
  if (!artifacts) {
    fs.rmSync(output, { force: true });
    process.stdout.write(
      "No signed updater artifacts found; latest.json was not generated.\n",
    );
    return;
  }
  verifyArtifactSignatures(assetsDir, artifacts, updaterPublicKey(options));

  const platforms = {};
  for (const target of Object.keys(TARGETS)) {
    const filename = artifacts[target];
    platforms[target] = {
      signature: fs
        .readFileSync(path.join(assetsDir, `${filename}.sig`), "utf8")
        .trim(),
      url: releaseUrl(repository, tag, filename),
    };
  }
  const manifest = {
    version,
    notes: fs.readFileSync(notesFile, "utf8"),
    pub_date: pubDate,
    platforms,
  };
  fs.writeFileSync(output, `${JSON.stringify(manifest, null, 2)}\n`);
  process.stdout.write(`Generated signed updater manifest at ${output}\n`);
}

function verify(options) {
  const assetsDir = path.resolve(requireOption(options, "assets-dir"));
  const manifestPath = path.resolve(requireOption(options, "manifest"));
  const version = requireOption(options, "version");
  const tag = requireOption(options, "tag");
  const repository = requireOption(options, "repository");
  validateReleaseMetadata(version, tag, repository);

  const artifacts = requireCompleteArtifacts(assetsDir, false);
  verifyArtifactSignatures(assetsDir, artifacts, updaterPublicKey(options));
  let manifest;
  try {
    manifest = JSON.parse(fs.readFileSync(manifestPath, "utf8"));
  } catch (error) {
    fail(`Invalid updater manifest JSON: ${error.message}`);
  }
  if (manifest.version !== version) {
    fail(`Unexpected updater version: ${manifest.version}`);
  }
  if (typeof manifest.notes !== "string") {
    fail("Updater manifest notes must be a string");
  }
  validatePubDate(manifest.pub_date);

  const manifestTargets = Object.keys(manifest.platforms ?? {}).sort();
  const expectedTargets = Object.keys(TARGETS).sort();
  if (JSON.stringify(manifestTargets) !== JSON.stringify(expectedTargets)) {
    fail(`Unexpected updater target set: ${manifestTargets.join(", ")}`);
  }

  for (const target of expectedTargets) {
    const filename = artifacts[target];
    const entry = manifest.platforms[target];
    const expectedUrl = releaseUrl(repository, tag, filename);
    if (entry.url !== expectedUrl) {
      fail(`Unexpected updater URL for ${target}: ${entry.url}`);
    }
    const signature = fs
      .readFileSync(path.join(assetsDir, `${filename}.sig`), "utf8")
      .trim();
    if (!isValidSignature(entry.signature) || entry.signature !== signature) {
      fail(`Invalid updater signature for ${target}`);
    }
  }
  process.stdout.write(`Verified signed updater manifest at ${manifestPath}\n`);
}

export { TARGETS, generate, verify };

if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(process.argv[1]).href
) {
  try {
    const { command, options } = parseArgs(process.argv.slice(2));
    if (command === "generate") {
      generate(options);
    } else {
      verify(options);
    }
  } catch (error) {
    process.stderr.write(`${error.message}\n`);
    process.exitCode = 1;
  }
}
