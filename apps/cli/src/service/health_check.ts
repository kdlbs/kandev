import { spawnSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";

import { DEFAULT_BACKEND_PORT } from "../constants";

export type LogDumper = () => void;

const DEFAULT_TIMEOUT_MS = 30_000;
const POLL_INTERVAL_MS = 500;

/**
 * Poll the kandev /health endpoint to confirm the freshly-installed service
 * actually came up. On success, the user gets immediate confirmation; on
 * failure, we dump the tail of the service logs so the diagnosis isn't a
 * scavenger hunt across `journalctl` / `tail`.
 *
 * Timeout is fixed at 30s — long enough to absorb a slow first launch
 * (binary unpacking, sqlite migration), short enough to fail fast when the
 * unit is genuinely broken.
 */
export async function waitForServiceHealth(
  port: number | undefined,
  dumpLogs: LogDumper,
): Promise<void> {
  const url = `http://localhost:${port ?? DEFAULT_BACKEND_PORT}/health`;
  const deadline = Date.now() + DEFAULT_TIMEOUT_MS;
  process.stderr.write(`[kandev] waiting for service to be healthy at ${url}\n`);

  while (Date.now() < deadline) {
    try {
      const res = await fetch(url);
      if (res.ok) {
        process.stderr.write(`[kandev] service is healthy\n`);
        return;
      }
    } catch {
      // not up yet; keep polling
    }
    await sleep(POLL_INTERVAL_MS);
  }

  process.stderr.write(`[kandev] service did not become healthy within 30s\n`);
  process.stderr.write(`[kandev] dumping recent service logs:\n`);
  dumpLogs();
  throw new Error(
    "kandev service was installed but the /health endpoint never responded. " +
      "Inspect the logs above and re-run 'kandev service install' once fixed.",
  );
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

/** Dump the last N lines of a systemd unit's logs via journalctl. */
export function dumpJournalctlLogs(opts: { unit: string; isSystem: boolean; lines: number }): void {
  const args = opts.isSystem
    ? ["-u", opts.unit, "-n", String(opts.lines), "--no-pager"]
    : ["--user-unit", opts.unit, "-n", String(opts.lines), "--no-pager"];
  spawnSync("journalctl", args, { stdio: "inherit" });
}

/** Dump the last N lines of launchd-managed log files via `tail`. */
export function dumpLaunchdLogs(opts: { logDir: string; lines: number }): void {
  const candidates = ["service.err", "service.out"]
    .map((name) => path.join(opts.logDir, name))
    .filter((p) => fs.existsSync(p));
  if (candidates.length === 0) {
    process.stderr.write(`[kandev]   (no logs found in ${opts.logDir})\n`);
    return;
  }
  spawnSync("tail", ["-n", String(opts.lines), ...candidates], { stdio: "inherit" });
}
