import type { ChildProcess } from "node:child_process";
import kill from "tree-kill";

export type ChildLike = { pid?: number | undefined };

const SHUTDOWN_TIMEOUT_MS = 10000;

export function createProcessSupervisor(): {
  children: ChildLike[];
  shutdown: (reason: string) => Promise<void>;
  attachSignalHandlers: () => void;
} {
  let shutdownPromise: Promise<void> | null = null;
  const children: ChildLike[] = [];

  const shutdown = async (reason: string) => {
    // If already shutting down, wait for the existing shutdown to complete
    if (shutdownPromise) {
      return shutdownPromise;
    }

    console.log(`[kandev] shutting down (${reason})...`);

    // Wait for all child processes to actually exit, not just for signal to be sent
    shutdownPromise = Promise.all(
      children
        .filter((child) => child.pid)
        .map((child) => waitForProcessExit(child, SHUTDOWN_TIMEOUT_MS)),
    ).then(() => {});

    return shutdownPromise;
  };

  const onSignal = (signal: NodeJS.Signals) => {
    void shutdown(`signal ${signal}`).then(() => process.exit(0));
  };

  const attachSignalHandlers = () => {
    process.on("SIGINT", onSignal);
    // SIGTERM is not available on Windows — only attach where supported
    if (process.platform !== "win32") {
      process.on("SIGTERM", onSignal);
    }
  };

  return { children, shutdown, attachSignalHandlers };
}

/**
 * Terminate a process and wait for it to exit.
 * On Unix: sends SIGTERM, falls back to SIGKILL after timeout.
 * On Windows: tree-kill uses taskkill (no SIGTERM/SIGKILL distinction).
 */
function waitForProcessExit(child: ChildLike, timeoutMs: number): Promise<void> {
  const isWindows = process.platform === "win32";

  return new Promise<void>((resolve) => {
    const pid = child.pid as number;

    // Check if this is a ChildProcess with exit event support
    const proc = child as ChildProcess;
    const hasExitEvent = typeof proc.on === "function" && typeof proc.exitCode !== "undefined";

    // If process already exited, resolve immediately
    if (hasExitEvent && proc.exitCode !== null) {
      resolve();
      return;
    }

    let resolved = false;
    const done = () => {
      if (resolved) return;
      resolved = true;
      resolve();
    };

    // Set up timeout for force-kill fallback
    const timeout = setTimeout(() => {
      console.log(`[kandev] process ${pid} did not exit in time, force killing`);
      // On Windows, tree-kill always force-kills (no signal distinction)
      // On Unix, escalate to SIGKILL
      kill(pid, isWindows ? undefined : "SIGKILL", done);
    }, timeoutMs);

    // Listen for exit event if available
    if (hasExitEvent) {
      proc.once("exit", () => {
        clearTimeout(timeout);
        done();
      });
    }

    // Graceful termination: SIGTERM on Unix, default kill on Windows
    kill(pid, isWindows ? undefined : "SIGTERM", (err) => {
      // If kill fails (process already gone), we're done
      if (err) {
        clearTimeout(timeout);
        done();
      }
      // If no exit event support, resolve after a brief delay
      // (tree-kill callback fires when signal sent, not when process exits)
      if (!hasExitEvent) {
        // For non-ChildProcess objects, we can't wait for exit event
        // Just wait for the timeout
      }
    });
  });
}
