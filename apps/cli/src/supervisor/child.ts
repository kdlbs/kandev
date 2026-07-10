import { spawn, type ChildProcess, type StdioOptions } from "node:child_process";
import kill from "tree-kill";

import type { LaunchManifest } from "./manifest";

const RESTART_TIMEOUT_MS = 10000;

export type RestartableChild = {
  current: () => ChildProcess | null;
  start: () => ChildProcess;
  restart: () => Promise<ChildProcess>;
  stop: () => Promise<void>;
};

export function createRestartableChild(
  manifest: LaunchManifest,
  options: { stdio?: StdioOptions; extraEnv?: NodeJS.ProcessEnv } = {},
): RestartableChild {
  let child: ChildProcess | null = null;

  const start = () => {
    child = spawn(manifest.backend_executable, manifest.argv, {
      cwd: manifest.cwd,
      env: {
        ...process.env,
        ...manifest.env,
        ...options.extraEnv,
      },
      stdio: options.stdio ?? "inherit",
    });
    return child;
  };

  const stop = async () => {
    if (!child?.pid || child.exitCode !== null) return;
    await terminate(child, RESTART_TIMEOUT_MS);
  };

  const restart = async () => {
    await stop();
    return start();
  };

  return {
    current: () => child,
    start,
    restart,
    stop,
  };
}

function terminate(proc: ChildProcess, timeoutMs: number): Promise<void> {
  return new Promise((resolve) => {
    const pid = proc.pid;
    if (!pid) {
      resolve();
      return;
    }
    let done = false;
    const finish = () => {
      if (done) return;
      done = true;
      clearTimeout(timeout);
      resolve();
    };
    const timeout = setTimeout(() => {
      kill(pid, process.platform === "win32" ? undefined : "SIGKILL", finish);
    }, timeoutMs);
    proc.once("exit", finish);
    kill(pid, process.platform === "win32" ? undefined : "SIGTERM", (err) => {
      if (err) finish();
    });
  });
}
