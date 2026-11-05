/**
 * Process-group ownership and cleanup helpers for the debug source-start check.
 */
import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const currentRepo = path.resolve(scriptDir, "../../../..");

const resolvedRepo = path.resolve(process.env.KANDEV_DEBUG_START_REPO || currentRepo);
export const repo = fs.existsSync(resolvedRepo)
  ? fs.realpathSync.native(resolvedRepo)
  : resolvedRepo;
export const POSIX = process.platform !== "win32";

/**
 * @param {number} ms
 */
export async function delay(ms) {
  await new Promise((resolve) => setTimeout(resolve, ms));
}

/**
 * Check whether a process group still has members.
 *
 * @param {number | undefined} pgid
 * @returns {boolean}
 */
export function processGroupAlive(pgid) {
  if (!POSIX || !pgid) return false;
  try {
    const table = readProcessTable();
    if (table.length > 0) {
      return table.some((entry) => entry.pgid === pgid && entry.stat !== "Z");
    }
    process.kill(-pgid, 0);
    return true;
  } catch (error) {
    return error.code === "EPERM";
  }
}

/**
 * Read the POSIX process table. `ps -axo` is available on Linux and macOS.
 *
 * @returns {Array<{pid: number, ppid: number, pgid: number, stat: string, command: string}>}
 */
export function readProcessTable() {
  if (!POSIX) return [];
  try {
    const output = execFileSync("ps", ["-axo", "pid=,ppid=,pgid=,stat=,command="], {
      encoding: "utf8",
    });
    return output
      .split("\n")
      .map((line) => {
        const match = line.trim().match(/^(\d+)\s+(\d+)\s+(\d+)\s+(\S+)\s+(.*)$/);
        if (!match) return null;
        return {
          pid: Number(match[1]),
          ppid: Number(match[2]),
          pgid: Number(match[3]),
          stat: match[4],
          command: match[5],
        };
      })
      .filter(Boolean);
  } catch {
    return [];
  }
}

/**
 * Expand a set of root pids to include every descendant in the table.
 *
 * @param {number[]} rootPids
 * @param {Array<{pid: number, ppid: number}>} table
 * @returns {number[]}
 */
export function collectDescendantPids(rootPids, table) {
  const result = new Set(rootPids.filter(Boolean));
  let changed = true;
  while (changed) {
    changed = false;
    for (const entry of table) {
      if (!result.has(entry.pid) && result.has(entry.ppid)) {
        result.add(entry.pid);
        changed = true;
      }
    }
  }
  return [...result];
}

/**
 * A backend is owned when its command names this checkout and the backend
 * entrypoint, and it leads its own managed process group.
 *
 * @param {{pid: number, pgid: number, command: string}} entry
 * @returns {boolean}
 */
export function isOwnedBackendProcess(entry) {
  return (
    entry.command.includes("__backend") && entry.command.includes(repo) && entry.pgid === entry.pid
  );
}

/**
 * Collect backend group leaders descended from the owned launcher roots.
 *
 * @param {number[]} rootPids
 * @param {Array<{pid: number, ppid: number, pgid: number, command: string}>} table
 * @returns {number[]}
 */
export function collectOwnedBackendPids(rootPids, table) {
  const descendants = collectDescendantPids(rootPids, table);
  return descendants
    .map((pid) => table.find((entry) => entry.pid === pid))
    .filter((entry) => entry && isOwnedBackendProcess(entry))
    .map((entry) => entry.pid);
}

/**
 * @param {number | undefined} pgid
 * @param {NodeJS.Signals} signal
 */
export function signalProcessGroup(pgid, signal) {
  if (!POSIX || !pgid) return;
  try {
    process.kill(-pgid, signal);
  } catch (error) {
    if (error.code !== "ESRCH" && error.code !== "EPERM") throw error;
  }
}

/**
 * @param {number | undefined} pgid
 * @param {number} timeoutMs
 */
export async function waitForGroupExit(pgid, timeoutMs) {
  const deadline = Date.now() + timeoutMs;
  while (processGroupAlive(pgid) && Date.now() < deadline) {
    await delay(250);
  }
}

/**
 * Stop only the launcher tree this check created.
 *
 * The outer make process owns a process group, while the native launcher puts
 * the backend in its own managed group. Signal both from owned identity
 * evidence, including when the outer make exits first or startup fails.
 *
 * @param {import("node:child_process").ChildProcess} child
 * @param {string} startLog
 */
export async function stopLauncher(child, startLog) {
  if (!POSIX) {
    child.kill("SIGTERM");
    return;
  }

  const loggedBackendPid = readBackendPid(startLog);
  const roots = [child.pid, readLauncherPid(startLog)].filter(Boolean);
  const backendPids = new Set(collectOwnedBackendPids(roots, readProcessTable()));
  if (loggedBackendPid) backendPids.add(loggedBackendPid);

  signalProcessGroup(child.pid, "SIGTERM");
  await waitForGroupExit(child.pid, 15_000);
  if (processGroupAlive(child.pid)) {
    signalProcessGroup(child.pid, "SIGKILL");
    await waitForGroupExit(child.pid, 5_000);
  }

  const table = readProcessTable();
  for (const pid of collectOwnedBackendPids(
    [child.pid, readLauncherPid(startLog), loggedBackendPid],
    table,
  )) {
    backendPids.add(pid);
  }
  const lateBackendPid = readBackendPid(startLog);
  if (lateBackendPid) backendPids.add(lateBackendPid);

  for (const pid of backendPids) {
    const entry = readProcessTable().find((item) => item.pid === pid);
    if (!entry || !isOwnedBackendProcess(entry)) continue;
    signalProcessGroup(pid, "SIGTERM");
    await waitForGroupExit(pid, 10_000);
    if (processGroupAlive(pid)) signalProcessGroup(pid, "SIGKILL");
    await waitForGroupExit(pid, 5_000);
  }
}

/**
 * @param {string} startLog
 * @returns {number | undefined}
 */
export function readLauncherPid(startLog) {
  try {
    const match = fs.readFileSync(startLog, "utf8").match(/launcher_pid=(\d+)/);
    return match ? Number(match[1]) : undefined;
  } catch {
    return undefined;
  }
}

/**
 * @param {string} startLog
 * @returns {number | undefined}
 */
export function readBackendPid(startLog) {
  try {
    const match = fs
      .readFileSync(startLog, "utf8")
      .match(/managed process started label="backend" pid=(\d+)/);
    return match ? Number(match[1]) : undefined;
  } catch {
    return undefined;
  }
}
