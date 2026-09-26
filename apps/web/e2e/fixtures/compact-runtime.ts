import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";

const REPO_ROOT = findRepoRoot();
const HELPER_ASSET_TOOL = path.join(REPO_ROOT, "scripts/release/remote-helper-assets.mjs");
const REMOTE_HELPER_NAMES = [
  "agentctl-linux-amd64",
  "agentctl-linux-arm64",
  "agentctl-darwin-amd64",
  "agentctl-darwin-arm64",
];

export type CompactRuntimeIdentity = {
  version: string;
  commit: string;
};

export type CompactRuntimeFixture = CompactRuntimeIdentity & {
  bundleDir: string;
  launcherPath: string;
  cacheRoot: string;
  cachePath: string;
  linuxHelperPath: string;
};

export type PrepareCompactRuntimeOptions = {
  bundleDir: string;
  homeDir: string;
  sourceBinDir: string;
  launcherPath?: string;
  identity?: CompactRuntimeIdentity;
};

/** Build a standard test bundle and seed its verified linux/amd64 helper cache. */
export function prepareCompactRuntimeFixture({
  bundleDir,
  homeDir,
  sourceBinDir,
  launcherPath = path.join(sourceBinDir, "kandev"),
  identity,
}: PrepareCompactRuntimeOptions): CompactRuntimeFixture {
  const parentDir = path.dirname(bundleDir);
  const agentctlPath = path.join(sourceBinDir, "agentctl");
  const linuxHelperPath = path.join(sourceBinDir, "agentctl-linux-amd64");
  for (const sourcePath of [launcherPath, agentctlPath, linuxHelperPath]) {
    if (!fs.statSync(sourcePath, { throwIfNoEntry: false })?.isFile()) {
      throw new Error(`Compact runtime E2E requires a built executable at ${sourcePath}`);
    }
  }
  const runtimeIdentity = identity ?? currentBuildIdentity(launcherPath, sourceBinDir);

  fs.mkdirSync(path.join(bundleDir, "bin"), { recursive: true });
  const packagedLauncher = path.join(bundleDir, "bin", "kandev");
  fs.copyFileSync(launcherPath, packagedLauncher);
  fs.copyFileSync(agentctlPath, path.join(bundleDir, "bin", "agentctl"));
  fs.chmodSync(packagedLauncher, 0o755);
  fs.chmodSync(path.join(bundleDir, "bin", "agentctl"), 0o755);

  const helperSourceDir = path.join(parentDir, "canonical-test-helpers");
  const helperArtifactDir = path.join(parentDir, "canonical-test-artifact");
  fs.mkdirSync(helperSourceDir, { recursive: true });
  for (const name of REMOTE_HELPER_NAMES) {
    const destination = path.join(helperSourceDir, name);
    fs.copyFileSync(linuxHelperPath, destination);
    fs.chmodSync(destination, 0o755);
  }
  execFileSync(
    process.execPath,
    [
      HELPER_ASSET_TOOL,
      "build",
      "--bin-dir",
      helperSourceDir,
      "--output-dir",
      helperArtifactDir,
      "--version",
      runtimeIdentity.version,
      "--commit",
      runtimeIdentity.commit,
      "--stable",
      "true",
    ],
    { cwd: REPO_ROOT, stdio: "pipe" },
  );

  const manifestPath = path.join(helperArtifactDir, "manifests", "standard", "remote-helpers.json");
  fs.copyFileSync(manifestPath, path.join(bundleDir, "remote-helpers.json"));
  execFileSync(
    process.execPath,
    [HELPER_ASSET_TOOL, "verify-bundle", "--bundle-dir", bundleDir, "--variant", "standard"],
    { cwd: REPO_ROOT, stdio: "pipe" },
  );

  const manifest = JSON.parse(fs.readFileSync(manifestPath, "utf8")) as {
    helpers: Array<{ platform: string; sha256: string }>;
  };
  const linuxRecord = manifest.helpers.find((record) => record.platform === "linux/amd64");
  if (!linuxRecord) throw new Error("Compact runtime manifest is missing linux/amd64");
  const cacheRoot = path.join(homeDir, "cache", "remote-helpers");
  const cachePath = path.join(
    cacheRoot,
    runtimeIdentity.version,
    "linux-amd64",
    linuxRecord.sha256,
    "agentctl",
  );
  fs.mkdirSync(path.dirname(cachePath), { recursive: true });
  fs.copyFileSync(linuxHelperPath, cachePath);
  fs.chmodSync(cachePath, 0o755);
  fs.rmSync(helperSourceDir, { recursive: true, force: true });
  fs.rmSync(helperArtifactDir, { recursive: true, force: true });

  return {
    ...runtimeIdentity,
    bundleDir,
    launcherPath: packagedLauncher,
    cacheRoot,
    cachePath,
    linuxHelperPath,
  };
}

function currentBuildIdentity(launcherPath: string, sourceBinDir: string): CompactRuntimeIdentity {
  return {
    version: execFileSync(launcherPath, ["--version"], {
      cwd: REPO_ROOT,
      encoding: "utf8",
    }).trim(),
    commit: buildSourceRevision(sourceBinDir) ?? gitHeadCommit(),
  };
}

function buildSourceRevision(sourceBinDir: string): string | undefined {
  const identityPath = path.join(sourceBinDir, "e2e-build-identity.json");
  if (!fs.existsSync(identityPath)) return undefined;

  const identity = JSON.parse(fs.readFileSync(identityPath, "utf8")) as {
    schema_version?: unknown;
    source_revision?: unknown;
  };
  if (
    identity.schema_version !== 1 ||
    typeof identity.source_revision !== "string" ||
    !/^[a-f0-9]{40}$/.test(identity.source_revision)
  ) {
    throw new Error(`Invalid E2E build identity at ${identityPath}`);
  }
  return identity.source_revision;
}

function gitHeadCommit(): string {
  return execFileSync("git", ["rev-parse", "HEAD"], {
    cwd: REPO_ROOT,
    encoding: "utf8",
  }).trim();
}

function findRepoRoot(startDir = process.cwd()): string {
  let currentDir = path.resolve(startDir);
  while (true) {
    if (
      fs.existsSync(path.join(currentDir, "scripts/release/remote-helper-assets.mjs")) &&
      fs.existsSync(path.join(currentDir, "apps/web/package.json"))
    ) {
      return currentDir;
    }
    const parentDir = path.dirname(currentDir);
    if (parentDir === currentDir) break;
    currentDir = parentDir;
  }
  throw new Error(`Cannot find the Kandev repository root from ${startDir}`);
}
