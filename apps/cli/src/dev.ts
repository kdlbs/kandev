import { spawn } from "node:child_process";
import path from "node:path";

import { HEALTH_TIMEOUT_MS_DEV } from "./constants";
import { resolveHealthTimeoutMs, waitForHealth, waitForUrlReady } from "./health";
import { createProcessSupervisor } from "./process";
import {
  attachBackendExitHandler,
  buildBackendEnv,
  buildWebEnv,
  logStartupInfo,
  pickPorts,
} from "./shared";
import { launchWebApp, openBrowser } from "./web";

export type DevOptions = {
  repoRoot: string;
  backendPort?: number;
  webPort?: number;
};

export async function runDev({ repoRoot, backendPort, webPort }: DevOptions): Promise<void> {
  const ports = await pickPorts(backendPort, webPort);
  const dbPath = path.join(repoRoot, "apps", "backend", "kandev.db");

  const backendEnv = buildBackendEnv({
    ports,
    extra: { KANDEV_MOCK_AGENT: "true", KANDEV_DATABASE_PATH: dbPath },
  });
  const webEnv = buildWebEnv({ ports, includeMcp: true, debug: true });
  const logLevel =
    process.env.KANDEV_LOGGING_LEVEL?.trim() || process.env.KANDEV_LOG_LEVEL?.trim() || "info";

  logStartupInfo({
    header: "dev mode: using local repo",
    ports,
    dbPath,
    includeMcp: true,
    logLevel,
  });

  const supervisor = createProcessSupervisor();
  supervisor.attachSignalHandlers();

  const backendProc = spawn("make", ["-C", path.join("apps", "backend"), "dev"], {
    cwd: repoRoot,
    env: backendEnv,
    stdio: "inherit",
  });
  supervisor.children.push(backendProc);

  attachBackendExitHandler(backendProc, supervisor);

  const healthTimeoutMs = resolveHealthTimeoutMs(HEALTH_TIMEOUT_MS_DEV);
  console.log("[kandev] starting backend...");
  await waitForHealth(ports.backendUrl, backendProc, healthTimeoutMs);
  console.log(`[kandev] backend ready at ${ports.backendUrl}`);

  const webUrl = `http://localhost:${ports.webPort}`;
  console.log("[kandev] starting web...");
  const webProc = launchWebApp({
    command: "pnpm",
    args: ["-C", "apps", "--filter", "@kandev/web", "dev"],
    cwd: repoRoot,
    env: webEnv,
    supervisor,
    label: "web",
  });
  await waitForUrlReady(webUrl, webProc, healthTimeoutMs);
  console.log(`[kandev] web ready at ${webUrl}`);
  openBrowser(webUrl);
}
