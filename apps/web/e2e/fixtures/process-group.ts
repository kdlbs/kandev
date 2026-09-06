import { execFile } from "node:child_process";
import type { ChildProcess } from "node:child_process";

type WindowsTreeKiller = (pid: number, done: (error?: Error) => void) => void;
type ProcessAliveProbe = (pid: number) => boolean;

const taskkillProcessTree: WindowsTreeKiller = (pid, done) => {
  execFile("taskkill", ["/PID", String(pid), "/T", "/F"], (error) => done(error ?? undefined));
};

const isProcessAlive: ProcessAliveProbe = (pid) => {
  try {
    process.kill(pid, 0);
    return true;
  } catch {
    return false;
  }
};

/**
 * Kills the backend and every child process it owns. POSIX uses the detached
 * process group; Windows needs taskkill because negative-PID signals are not
 * supported there.
 */
export function killProcessGroup(
  proc: ChildProcess,
  platform: NodeJS.Platform = process.platform,
  killWindowsTree: WindowsTreeKiller = taskkillProcessTree,
  processIsAlive: ProcessAliveProbe = isProcessAlive,
): Promise<void> {
  return new Promise<void>((resolve, reject) => {
    if (!proc.pid) {
      resolve();
      return;
    }

    const pid = proc.pid;

    if (platform === "win32") {
      killWindowsTree(pid, (error) => {
        if (error && processIsAlive(pid)) {
          reject(error);
          return;
        }
        resolve();
      });
      return;
    }

    try {
      process.kill(-pid, "SIGTERM");
    } catch {
      // Process group may already be gone
      resolve();
      return;
    }

    const timeout = setTimeout(() => {
      try {
        process.kill(-pid, "SIGKILL");
      } catch {
        // Already dead
      }
      resolve();
    }, 7_000);

    proc.on("exit", () => {
      clearTimeout(timeout);
      resolve();
    });
  });
}
