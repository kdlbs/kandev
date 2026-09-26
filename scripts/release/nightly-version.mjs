#!/usr/bin/env node

import { pathToFileURL } from "node:url";

const STABLE_SEMVER = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/;
const FULL_GIT_SHA = /^[0-9a-f]{40}$/;

export function nightlyVersion(stableVersion, commitSha) {
  const match = STABLE_SEMVER.exec(stableVersion);
  if (match === null) {
    throw new Error(`Expected a stable SemVer X.Y.Z, received: ${stableVersion}`);
  }
  if (!FULL_GIT_SHA.test(commitSha)) {
    throw new Error("Expected a 40-character lowercase hexadecimal Git commit SHA");
  }

  const [, major, minor, patch] = match;
  return `${major}.${minor}.${BigInt(patch) + 1n}-nightly.sha${commitSha.slice(0, 12)}`;
}

function main(args) {
  if (args.length !== 2) {
    throw new Error("Usage: nightly-version.mjs <stable-version> <40-character-commit-sha>");
  }
  process.stdout.write(`${nightlyVersion(args[0], args[1])}\n`);
}

if (process.argv[1] !== undefined && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    main(process.argv.slice(2));
  } catch (error) {
    process.stderr.write(`${error instanceof Error ? error.message : String(error)}\n`);
    process.exitCode = 1;
  }
}
