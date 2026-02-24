import { spawn } from "node:child_process";
import fs from "node:fs";
import path from "node:path";

import { ensureExtracted, findBundleRoot, resolveWebServerPath } from "./bundle";
import {
  DATA_DIR,
  DEFAULT_AGENTCTL_PORT,
  DEFAULT_BACKEND_PORT,
  DEFAULT_WEB_PORT,
  HEALTH_TIMEOUT_MS_RELEASE,
} from "./constants";
import { ensureAsset, getRelease } from "./github";
import { CACHE_DIR } from "./constants";
import { resolveHealthTimeoutMs, waitForHealth } from "./health";
import { getBinaryName, getPlatformDir } from "./platform";
import { pickAvailablePort } from "./ports";
import { createProcessSupervisor } from "./process";
import { attachBackendExitHandler } from "./shared";
import { launchWebApp } from "./web";

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
  webPort: number;
  agentctlPort: number;
};

async function prepareReleaseBundle({
  version,
  backendPort,
  webPort,
}: RunOptions): Promise<PreparedRelease> {
  const platformDir = getPlatformDir();
  const release = await getRelease(version);
  const tag = release.tag_name || "latest";
  const assetName = `kandev-${platformDir}.tar.gz`;
  const cacheDir = path.join(CACHE_DIR, tag, platformDir);

  const archivePath = await ensureAsset(release, assetName, cacheDir, (downloaded, total) => {
    const percent = total ? Math.round((downloaded / total) * 100) : 0;
    const mb = (downloaded / (1024 * 1024)).toFixed(1);
    const totalMb = total ? (total / (1024 * 1024)).toFixed(1) : "?";
    process.stderr.write(`\r   Downloading: ${mb}MB / ${totalMb}MB (${percent}%)`);
  });
  process.stderr.write("\n");

  ensureExtracted(archivePath, cacheDir);
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
  // Log level is set to warn for clean production output.
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
    webPort: actualWebPort,
    agentctlPort,
  };
}

function launchReleaseApps(prepared: PreparedRelease): {
  supervisor: ReturnType<typeof createProcessSupervisor>;
  backendProc: ReturnType<typeof spawn>;
  webServerPath: string;
} {
  console.log("[kandev] backend port:", prepared.backendEnv.KANDEV_SERVER_PORT);
  console.log("[kandev] web port:", prepared.webPort);
  console.log("[kandev] agentctl port:", prepared.agentctlPort);

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

  const webUrl = `http://localhost:${prepared.webPort}`;
  launchWebApp({
    command: "node",
    args: [webServerPath],
    cwd: path.dirname(webServerPath),
    env: prepared.webEnv,
    url: webUrl,
    supervisor,
    label: "web",
  });

  return { supervisor, backendProc, webServerPath };
}

export async function runRelease({ version, backendPort, webPort }: RunOptions): Promise<void> {
  const prepared = await prepareReleaseBundle({ version, backendPort, webPort });
  const { backendProc } = launchReleaseApps(prepared);
  // Wait for backend before announcing the web URL.
  const healthTimeoutMs = resolveHealthTimeoutMs(HEALTH_TIMEOUT_MS_RELEASE);
  await waitForHealth(prepared.backendUrl, backendProc, healthTimeoutMs);
  console.log(`[kandev] backend ready at ${prepared.backendUrl}`);
  console.log(`[kandev] web ready at http://localhost:${prepared.webPort}`);
}
