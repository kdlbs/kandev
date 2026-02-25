import { spawn } from "node:child_process";
import fs from "node:fs";
import path from "node:path";

import { ensureExtracted, findBundleRoot, resolveWebServerPath } from "./bundle";
import {
  CACHE_DIR,
  DATA_DIR,
  DEFAULT_AGENTCTL_PORT,
  DEFAULT_BACKEND_PORT,
  DEFAULT_WEB_PORT,
  HEALTH_TIMEOUT_MS_RELEASE,
} from "./constants";
import { ensureAsset, getRelease } from "./github";
import { resolveHealthTimeoutMs, waitForHealth, waitForUrlReady } from "./health";
import { getBinaryName, getPlatformDir } from "./platform";
import { sortVersionsDesc } from "./version";
import { pickAvailablePort } from "./ports";
import { createProcessSupervisor } from "./process";
import { attachBackendExitHandler, logStartupInfo } from "./shared";
import { launchWebApp, openBrowser } from "./web";

export type RunOptions = {
  version?: string;
  backendPort?: number;
  webPort?: number;
};

type PreparedRelease = {
  bundleDir: string;
  backendBin: string;
  backendUrl: string;
  backendEnv: NodeJS.ProcessEnv;
  webEnv: NodeJS.ProcessEnv;
  releaseTag: string;
  requestedVersion?: string;
  webPort: number;
  agentctlPort: number;
  dbPath: string;
  logLevel: string;
};

/**
 * Find a cached release binary to use when GitHub is unreachable.
 * If version is specified, checks that exact tag. Otherwise, picks
 * the highest semver tag available in the cache.
 */
export function findCachedRelease(
  platformDir: string,
  version?: string,
): { cacheDir: string; tag: string } | null {
  if (version) {
    const cacheDir = path.join(CACHE_DIR, version, platformDir);
    const bundleDir = path.join(cacheDir, "kandev");
    const backendBin = path.join(bundleDir, "bin", getBinaryName("kandev"));
    if (fs.existsSync(backendBin)) {
      return { cacheDir, tag: version };
    }
    return null;
  }

  // No version specified — scan for cached tags and pick the latest.
  if (!fs.existsSync(CACHE_DIR)) return null;

  const entries = fs.readdirSync(CACHE_DIR).filter((d) => d.startsWith("v"));
  if (entries.length === 0) return null;

  const sorted = sortVersionsDesc(entries);

  for (const tag of sorted) {
    const cacheDir = path.join(CACHE_DIR, tag, platformDir);
    const bundleDir = path.join(cacheDir, "kandev");
    const backendBin = path.join(bundleDir, "bin", getBinaryName("kandev"));
    if (fs.existsSync(backendBin)) {
      return { cacheDir, tag };
    }
  }

  return null;
}

/**
 * Remove old cached releases, keeping only the 2 most recent tags.
 * Runs after a successful download so we don't accumulate stale versions.
 * The previous version is kept as a fallback for offline use.
 */
export function cleanOldReleases(currentTag: string) {
  try {
    if (!fs.existsSync(CACHE_DIR)) return;
    const entries = fs.readdirSync(CACHE_DIR).filter((d) => d.startsWith("v"));
    if (entries.length <= 2) return;

    const sorted = sortVersionsDesc(entries);

    // Always keep currentTag + the next most recent.
    const keep = new Set<string>([currentTag, sorted[0], sorted[1]]);
    for (const entry of entries) {
      if (!keep.has(entry)) {
        fs.rmSync(path.join(CACHE_DIR, entry), { recursive: true, force: true });
      }
    }
  } catch {
    // Non-critical — don't fail the launch if cleanup errors.
  }
}

async function prepareReleaseBundle({
  version,
  backendPort,
  webPort,
}: RunOptions): Promise<PreparedRelease> {
  const platformDir = getPlatformDir();
  let tag: string;
  let cacheDir: string;

  try {
    const release = await getRelease(version);
    tag = release.tag_name;
    const assetName = `kandev-${platformDir}.tar.gz`;
    cacheDir = path.join(CACHE_DIR, tag, platformDir);

    const archivePath = await ensureAsset(tag, assetName, cacheDir, (downloaded, total) => {
      const percent = total ? Math.round((downloaded / total) * 100) : 0;
      const mb = (downloaded / (1024 * 1024)).toFixed(1);
      const totalMb = total ? (total / (1024 * 1024)).toFixed(1) : "?";
      process.stderr.write(`\r   Downloading: ${mb}MB / ${totalMb}MB (${percent}%)`);
    });
    process.stderr.write("\n");

    ensureExtracted(archivePath, cacheDir);
    cleanOldReleases(tag);
  } catch (err) {
    // GitHub unreachable — try to launch from cache.
    const cached = findCachedRelease(platformDir, version);
    if (!cached) {
      const target = version ? `version ${version}` : "latest version";
      const reason = err instanceof Error ? err.message : String(err);
      throw new Error(
        `Failed to fetch ${target} and no cached release found.\n` +
          `  Reason: ${reason}\n` +
          `  Run kandev once while online to cache a release for offline use.`,
      );
    }
    tag = cached.tag;
    cacheDir = cached.cacheDir;
    process.stderr.write(`[kandev] GitHub unreachable — using cached release ${tag}\n`);
  }

  const bundleDir = findBundleRoot(cacheDir);

  const backendBin = path.join(bundleDir, "bin", getBinaryName("kandev"));
  if (!fs.existsSync(backendBin)) {
    throw new Error(`Backend binary not found at ${backendBin}`);
  }

  const agentctlBin = path.join(bundleDir, "bin", getBinaryName("agentctl"));
  if (!fs.existsSync(agentctlBin)) {
    throw new Error(`agentctl binary not found at ${agentctlBin}`);
  }

  const actualBackendPort = backendPort ?? (await pickAvailablePort(DEFAULT_BACKEND_PORT));
  const actualWebPort = webPort ?? (await pickAvailablePort(DEFAULT_WEB_PORT));
  const agentctlPort = await pickAvailablePort(DEFAULT_AGENTCTL_PORT);
  const backendUrl = `http://localhost:${actualBackendPort}`;
  const logLevel = process.env.KANDEV_LOG_LEVEL?.trim() || "warn";

  fs.mkdirSync(DATA_DIR, { recursive: true });
  const dbPath = path.join(DATA_DIR, "kandev.db");

  // Note: Release mode doesn't configure MCP server ports as it uses
  // the bundled configuration. Only backend and agentctl ports are set.
  // Log level defaults to warn for clean output and can be overridden
  // via KANDEV_LOG_LEVEL.
  const backendEnv = {
    ...process.env,
    KANDEV_SERVER_PORT: String(actualBackendPort),
    KANDEV_AGENT_STANDALONE_PORT: String(agentctlPort),
    KANDEV_DATABASE_PATH: dbPath,
    KANDEV_LOG_LEVEL: logLevel,
  };

  const webEnv: NodeJS.ProcessEnv = {
    ...process.env,
    KANDEV_API_BASE_URL: backendUrl,
    NEXT_PUBLIC_KANDEV_API_BASE_URL: backendUrl,
    PORT: String(actualWebPort),
  };
  (webEnv as Record<string, string>).NODE_ENV = "production";

  return {
    bundleDir,
    backendBin,
    backendUrl,
    backendEnv,
    webEnv,
    releaseTag: tag,
    requestedVersion: version,
    webPort: actualWebPort,
    agentctlPort,
    dbPath,
    logLevel,
  };
}

function launchReleaseApps(prepared: PreparedRelease): {
  supervisor: ReturnType<typeof createProcessSupervisor>;
  backendProc: ReturnType<typeof spawn>;
  webServerPath: string;
} {
  const releaseSource = prepared.requestedVersion
    ? `(requested: ${prepared.requestedVersion})`
    : "(github latest)";
  logStartupInfo({
    header: `release: ${prepared.releaseTag} ${releaseSource}`,
    ports: {
      backendPort: Number(prepared.backendEnv.KANDEV_SERVER_PORT),
      webPort: prepared.webPort,
      agentctlPort: prepared.agentctlPort,
      mcpPort: 0,
      backendUrl: prepared.backendUrl,
      mcpUrl: "",
    },
    dbPath: prepared.dbPath,
    logLevel: prepared.logLevel,
  });

  const supervisor = createProcessSupervisor();
  supervisor.attachSignalHandlers();

  const backendProc = spawn(prepared.backendBin, [], {
    cwd: path.dirname(prepared.backendBin),
    env: prepared.backendEnv,
    stdio: "inherit",
  });
  supervisor.children.push(backendProc);

  attachBackendExitHandler(backendProc, supervisor);

  const webServerPath = resolveWebServerPath(prepared.bundleDir);
  if (!webServerPath) {
    throw new Error("Web server entry (server.js) not found in bundle");
  }

  return { supervisor, backendProc, webServerPath };
}

export async function runRelease({ version, backendPort, webPort }: RunOptions): Promise<void> {
  const prepared = await prepareReleaseBundle({ version, backendPort, webPort });
  const { supervisor, backendProc, webServerPath } = launchReleaseApps(prepared);
  const healthTimeoutMs = resolveHealthTimeoutMs(HEALTH_TIMEOUT_MS_RELEASE);
  console.log("[kandev] starting backend...");
  await waitForHealth(prepared.backendUrl, backendProc, healthTimeoutMs);
  console.log(`[kandev] backend ready at ${prepared.backendUrl}`);

  const webUrl = `http://localhost:${prepared.webPort}`;
  console.log("[kandev] starting web...");
  const webProc = launchWebApp({
    command: "node",
    args: [webServerPath],
    cwd: path.dirname(webServerPath),
    env: prepared.webEnv,
    supervisor,
    label: "web",
    quiet: true,
  });
  await waitForUrlReady(webUrl, webProc, healthTimeoutMs);
  console.log(`[kandev] web ready at ${webUrl}`);
  openBrowser(webUrl);
}
