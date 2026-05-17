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
import { resolveRuntime, validateBundle } from "./runtime";
import { attachBackendExitHandler, logStartupInfo } from "./shared";
import { launchWebApp, openBrowser } from "./web";

export type RunOptions = {
  runtimeVersion?: string;
  backendPort?: number;
  webPort?: number;
  /** Show info logs from backend + web */
  verbose?: boolean;
  /** Show debug logs + agent message dumps */
  debug?: boolean;
  /** Skip browser open. Set by service units. */
  headless?: boolean;
};

type PreparedBundle = {
  bundleDir: string;
  backendBin: string;
  backendUrl: string;
  backendEnv: NodeJS.ProcessEnv;
  webEnv: NodeJS.ProcessEnv;
  releaseTag: string;
  webPort: number;
  agentctlPort: number;
  dbPath: string;
  logLevel: string;
  showOutput: boolean;
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

/**
 * Download a specific release version into the local cache.
 * Only used when --runtime-version is given explicitly.
 */
async function downloadRuntimeVersion(runtimeVersion: string): Promise<string> {
  const platformDir = getPlatformDir();
  const release = await getRelease(runtimeVersion);
  const tag = release.tag_name;
  const assetName = `kandev-${platformDir}.tar.gz`;
  const cacheDir = path.join(CACHE_DIR, tag, platformDir);

  const archivePath = await ensureAsset(tag, assetName, cacheDir, (downloaded, total) => {
    const percent = total ? Math.round((downloaded / total) * 100) : 0;
    const mb = (downloaded / (1024 * 1024)).toFixed(1);
    const totalMb = total ? (total / (1024 * 1024)).toFixed(1) : "?";
    process.stderr.write(`\r   Downloading: ${mb}MB / ${totalMb}MB (${percent}%)`);
  });
  process.stderr.write("\n");

  ensureExtracted(archivePath, cacheDir);
  cleanOldReleases(tag);
  return tag;
}

async function prepareBundleForLaunch({
  runtimeVersion,
  backendPort,
  webPort,
  verbose = false,
  debug = false,
}: RunOptions): Promise<PreparedBundle> {
  let bundleDir: string;
  let releaseTag: string;

  if (runtimeVersion) {
    // Explicit version: ensure it is in the cache (downloading if needed), then resolve.
    const platformDir = getPlatformDir();
    const cached = findCachedRelease(platformDir, runtimeVersion);
    let tag: string;
    if (cached) {
      tag = cached.tag;
      bundleDir = findBundleRoot(cached.cacheDir);
    } else {
      try {
        tag = await downloadRuntimeVersion(runtimeVersion);
        const cacheDir = path.join(CACHE_DIR, tag, platformDir);
        bundleDir = findBundleRoot(cacheDir);
      } catch (err) {
        const reason = err instanceof Error ? err.message : String(err);
        throw new Error(
          `Failed to fetch runtime version ${runtimeVersion}.\n` +
            `  Reason: ${reason}\n` +
            `  Run kandev once while online to cache a release for offline use.`,
        );
      }
    }
    // Validate the resolved bundle has all required binaries before launching.
    validateBundle(bundleDir);
    releaseTag = tag;
  } else {
    // Default path: resolve from KANDEV_BUNDLE_DIR or installed npm runtime package.
    const resolved = resolveRuntime();
    bundleDir = resolved.bundleDir;
    // Use KANDEV_VERSION if set (e.g. by Homebrew wrapper), otherwise show source.
    releaseTag = process.env.KANDEV_VERSION ?? `(${resolved.source})`;
  }

  const actualBackendPort = backendPort ?? (await pickAvailablePort(DEFAULT_BACKEND_PORT));
  const actualWebPort = webPort ?? (await pickAvailablePort(DEFAULT_WEB_PORT));
  const agentctlPort = await pickAvailablePort(DEFAULT_AGENTCTL_PORT);
  const backendUrl = `http://localhost:${actualBackendPort}`;
  const showOutput = verbose || debug;
  const logLevel =
    process.env.KANDEV_LOG_LEVEL?.trim() || (debug ? "debug" : verbose ? "info" : "warn");

  fs.mkdirSync(DATA_DIR, { recursive: true });
  const dbPath = path.join(DATA_DIR, "kandev.db");

  const backendBin = path.join(bundleDir, "bin", getBinaryName("kandev"));

  const backendEnv: NodeJS.ProcessEnv = {
    ...process.env,
    KANDEV_SERVER_PORT: String(actualBackendPort),
    KANDEV_WEB_INTERNAL_URL: `http://localhost:${actualWebPort}`,
    KANDEV_AGENT_STANDALONE_PORT: String(agentctlPort),
    KANDEV_DATABASE_PATH: dbPath,
    KANDEV_LOG_LEVEL: logLevel,
    ...(debug ? { KANDEV_DEBUG_AGENT_MESSAGES: "true", KANDEV_DEBUG_PPROF_ENABLED: "true" } : {}),
  };

  const webEnv: NodeJS.ProcessEnv = {
    ...process.env,
    KANDEV_API_BASE_URL: backendUrl,
    PORT: String(actualWebPort),
    HOSTNAME: "127.0.0.1",
  };
  (webEnv as Record<string, string>).NODE_ENV = "production";

  return {
    bundleDir,
    backendBin,
    backendUrl,
    backendEnv,
    webEnv,
    releaseTag,
    webPort: actualWebPort,
    agentctlPort,
    dbPath,
    logLevel,
    showOutput,
  };
}

/**
 * Attach a ring buffer to a readable stream, keeping roughly the last `maxChars`
 * characters. Note: the limit is measured in JS string length (UTF-16 code units),
 * not bytes — fine for log output which is overwhelmingly ASCII.
 */
export function attachRingBuffer(
  stream: NodeJS.ReadableStream | null,
  maxChars = 64 * 1024,
): () => string {
  let buf = "";
  stream?.on("data", (chunk: Buffer | string) => {
    buf += typeof chunk === "string" ? chunk : chunk.toString("utf8");
    if (buf.length > maxChars) {
      buf = buf.slice(buf.length - maxChars);
    }
  });
  return () => buf;
}

function launchBundle(prepared: PreparedBundle): {
  supervisor: ReturnType<typeof createProcessSupervisor>;
  backendProc: ReturnType<typeof spawn>;
  webServerPath: string;
  dumpBackendLogs: () => void;
} {
  logStartupInfo({
    header: `release: ${prepared.releaseTag}`,
    ports: {
      backendPort: Number(prepared.backendEnv.KANDEV_SERVER_PORT),
      webPort: prepared.webPort,
      agentctlPort: prepared.agentctlPort,
      backendUrl: prepared.backendUrl,
    },
    dbPath: prepared.dbPath,
    logLevel: prepared.logLevel,
  });

  const supervisor = createProcessSupervisor();
  supervisor.attachSignalHandlers();

  const backendProc = spawn(prepared.backendBin, [], {
    cwd: path.dirname(prepared.backendBin),
    env: prepared.backendEnv,
    stdio: prepared.showOutput ? ["ignore", "inherit", "inherit"] : ["ignore", "pipe", "inherit"],
  });
  supervisor.children.push(backendProc);

  const readBuffered = prepared.showOutput ? () => "" : attachRingBuffer(backendProc.stdout);
  let dumped = false;
  const dumpBackendLogs = (): void => {
    if (dumped) return;
    dumped = true;
    const buffered = readBuffered();
    if (buffered.trim().length === 0) return;
    console.error("[kandev] --- backend stdout (last captured output) ---");
    console.error(buffered.trimEnd());
    console.error("[kandev] --- end backend stdout ---");
  };

  attachBackendExitHandler(backendProc, supervisor);

  const webServerPath = resolveWebServerPath(prepared.bundleDir);
  if (!webServerPath) {
    throw new Error("Web server entry (server.js) not found in bundle");
  }

  return { supervisor, backendProc, webServerPath, dumpBackendLogs };
}

export async function runRelease({
  runtimeVersion,
  backendPort,
  webPort,
  verbose = false,
  debug = false,
  headless = false,
}: RunOptions): Promise<void> {
  const prepared = await prepareBundleForLaunch({
    runtimeVersion,
    backendPort,
    webPort,
    verbose,
    debug,
  });
  const { supervisor, backendProc, webServerPath, dumpBackendLogs } = launchBundle(prepared);
  const healthTimeoutMs = resolveHealthTimeoutMs(HEALTH_TIMEOUT_MS_RELEASE);
  console.log("[kandev] starting backend...");
  await waitForHealth(prepared.backendUrl, backendProc, healthTimeoutMs, dumpBackendLogs);
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
    quiet: !prepared.showOutput,
  });
  await waitForUrlReady(webUrl, webProc, healthTimeoutMs);
  if (headless) {
    console.log(`[kandev] ready (headless) at ${prepared.backendUrl}`);
    return;
  }
  console.log("[kandev] open: " + prepared.backendUrl);
  openBrowser(prepared.backendUrl);
}
