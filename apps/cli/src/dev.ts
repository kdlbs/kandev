import { spawn } from "node:child_process";
import fs from "node:fs";
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
  const webUrl = `http://localhost:${ports.webPort}`;

  logStartupInfo({
    header: "dev mode: using local repo",
    ports,
    dbPath,
    logLevel,
  });

  const supervisor = createProcessSupervisor();
  supervisor.attachSignalHandlers();

  const { cmd: backendCmd, args: backendArgs } = withWinjobWrap(repoRoot, "make", [
    "-C",
    path.join("apps", "backend"),
    "dev",
  ]);
  const backendProc = spawn(backendCmd, backendArgs, {
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
  console.log(`[kandev] open: ${webUrl}`);
  openBrowser(webUrl);
}

type DevBackendEnv = {
  dbPath: string;
  extra: Record<string, string>;
};

// Computes the dev-mode backend env. Dev mode always roots kandev under
// <repo>/.kandev-dev so state is isolated from the user's production ~/.kandev
// and so `make clean-db` (which removes .kandev-dev/) matches what `make dev`
// writes.
//
// When invoked from inside a kandev task workspace, any KANDEV_DATABASE_PATH
// is assumed to be leaked from the parent backend and is ignored. In a normal
// shell, an explicit KANDEV_DATABASE_PATH is honored as an escape hatch.
export function resolveDevBackendEnv(repoRoot: string): DevBackendEnv {
  const baseExtra: Record<string, string> = {
    KANDEV_MOCK_AGENT: process.env.KANDEV_MOCK_AGENT || "true",
    KANDEV_DEBUG_PPROF_ENABLED: "true",
  };
  const devHome = devKandevHome(repoRoot);
  // Display only; the backend derives its own DB path from KANDEV_HOME_DIR
  // via ResolvedDataDir(). Both resolve to the same location.
  const devDbPath = path.join(devHome, "data", "kandev.db");

  if (isInsideKandevTask(repoRoot)) {
    console.log("[kandev] task workspace detected → using local dev state");
    return {
      dbPath: devDbPath,
      extra: {
        ...baseExtra,
        KANDEV_HOME_DIR: devHome,
        // Clear a parent-leaked DB path so the backend uses the HomeDir-derived default.
        KANDEV_DATABASE_PATH: "",
      },
    };
  }

  const override = process.env.KANDEV_DATABASE_PATH;
  if (override) {
    return {
      dbPath: override,
      extra: { ...baseExtra, KANDEV_DATABASE_PATH: override },
    };
  }
  return {
    dbPath: devDbPath,
    extra: {
      ...baseExtra,
      KANDEV_HOME_DIR: devHome,
      KANDEV_DATABASE_PATH: "",
    },
  };
}

// withWinjobWrap on Windows prepends apps/backend/bin/winjob.exe to a spawn
// command so the child runs inside a Job Object configured with
// JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE. That makes "kill the whole backend
// subtree" a kernel-level guarantee tied to winjob's process exit, instead of
// relying on the bash → make → pnpm → node → make → sh → kandev signal chain
// (which drops Ctrl-C at multiple links because MSYS bash, native Win32
// processes, and Node disagree on signal propagation semantics).
//
// On Unix this is a passthrough — POSIX process groups already give us
// reliable cascading termination.
//
// If the winjob binary isn't built yet (the user ran `make dev` before
// `make -C apps/backend build-winjob`), we fall back to a direct spawn and
// emit a one-line note. The supervisor's tree-kill still handles the happy
// path; users only notice the gap if Ctrl-C drops mid-chain.
function withWinjobWrap(
  repoRoot: string,
  cmd: string,
  args: string[],
): { cmd: string; args: string[] } {
  if (process.platform !== "win32") return { cmd, args };
  const winjob = path.join(repoRoot, "apps", "backend", "bin", "winjob.exe");
  if (!fs.existsSync(winjob)) {
    console.warn(
      `[kandev] winjob.exe not built — Ctrl-C may leak processes on Windows. ` +
        `Run \`make -C apps/backend build-winjob\` once to enable.`,
    );
    return { cmd, args };
  }
  return { cmd: winjob, args: [cmd, ...args] };
}
