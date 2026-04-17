import { spawn } from "node:child_process";
import path from "node:path";

import { devKandevHome, HEALTH_TIMEOUT_MS_DEV } from "./constants";
import { resolveHealthTimeoutMs, waitForHealth, waitForUrlReady } from "./health";
import { isInsideKandevTask } from "./kandev-env";
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
  const { dbPath, extra } = resolveDevBackendEnv(repoRoot);
  const backendEnv = buildBackendEnv({ ports, extra });
  const webEnv = buildWebEnv({ ports, debug: true });
  const logLevel =
    process.env.KANDEV_LOGGING_LEVEL?.trim() || process.env.KANDEV_LOG_LEVEL?.trim() || "info";

  logStartupInfo({
    header: "dev mode: using local repo",
    ports,
    dbPath,
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

type DevBackendEnv = {
  dbPath: string;
  extra: Record<string, string>;
};

// Computes the dev-mode backend env. When invoked from inside a kandev task
// workspace, force-relocates the entire kandev root to <repo>/.kandev-dev so
// dev state is isolated from the user's production ~/.kandev. In a normal
// shell, preserves the existing override-friendly behavior (honor an
// explicit KANDEV_DATABASE_PATH, otherwise default to a repo-local db).
function resolveDevBackendEnv(repoRoot: string): DevBackendEnv {
  const baseExtra: Record<string, string> = {
    KANDEV_MOCK_AGENT: process.env.KANDEV_MOCK_AGENT || "true",
    KANDEV_DEBUG_PPROF_ENABLED: "true",
  };

  if (isInsideKandevTask(repoRoot)) {
    const devHome = devKandevHome(repoRoot);
    // Display only; the backend derives its own DB path from KANDEV_HOME_DIR
    // via ResolvedDataDir(). Both resolve to the same location.
    const dbPath = path.join(devHome, "data", "kandev.db");
    console.log("[kandev] task workspace detected → using local dev state");
    return {
      dbPath,
      extra: {
        ...baseExtra,
        KANDEV_HOME_DIR: devHome,
        // Clear a parent-leaked DB path so the backend uses the HomeDir-derived default.
        KANDEV_DATABASE_PATH: "",
      },
    };
  }

  const dbPath =
    process.env.KANDEV_DATABASE_PATH || path.join(repoRoot, "apps", "backend", "kandev.db");
  return {
    dbPath,
    extra: { ...baseExtra, KANDEV_DATABASE_PATH: dbPath },
  };
}
