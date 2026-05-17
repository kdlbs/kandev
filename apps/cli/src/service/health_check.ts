import { spawnSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";

import { DEFAULT_BACKEND_PORT } from "../constants";
import { delay, resolveHealthTimeoutMs } from "../health";

export type LogDumper = () => void;

const DEFAULT_TIMEOUT_MS = 30_000;
const POLL_INTERVAL_MS = 500;
// Per-request timeout. Without this, undici's default headersTimeout (5min)
// can stall a single fetch — e.g. TCP accepted but the backend hangs before
// writing response headers — and silently overrun the outer deadline.
const REQUEST_TIMEOUT_MS = 2_000;

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
  const timeoutMs = resolveHealthTimeoutMs(DEFAULT_TIMEOUT_MS);
  const deadline = Date.now() + timeoutMs;
  process.stderr.write(`[kandev] waiting for service to be healthy at ${url}\n`);

  while (Date.now() < deadline) {
    try {
      const res = await fetch(url, { signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS) });
      if (res.ok) {
        process.stderr.write(`[kandev] service is healthy\n`);
        return;
      }
    } catch {
      // not up yet (or per-request timeout fired); keep polling
    }
    await delay(POLL_INTERVAL_MS);
  }

  process.stderr.write(`[kandev] service did not become healthy within ${timeoutMs}ms\n`);
  process.stderr.write(`[kandev] dumping recent service logs:\n`);
  dumpLogs();
  throw new Error(
    "kandev service was installed but the /health endpoint never responded. " +
      "Inspect the logs above and re-run 'kandev service install' once fixed. " +
      "If the service needs more time to come up, set KANDEV_HEALTH_TIMEOUT_MS=120000.",
  );
}

/** Dump the last N lines of a systemd unit's logs via journalctl. */
export function dumpJournalctlLogs(opts: { unit: string; isSystem: boolean; lines: number }): void {
  const args = opts.isSystem
    ? ["-u", opts.unit, "-n", String(opts.lines), "--no-pager"]
    : ["--user-unit", opts.unit, "-n", String(opts.lines), "--no-pager"];
  try {
    spawnSync("journalctl", args, { stdio: "inherit" });
  } catch (err) {
    // journalctl may be unavailable in containerized or stripped-down setups.
    // We're already in a failure path; don't compound it with a thrown error.
    process.stderr.write(
      `[kandev]   (could not run journalctl: ${err instanceof Error ? err.message : String(err)})\n`,
    );
  }
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
  try {
    spawnSync("tail", ["-n", String(opts.lines), ...candidates], { stdio: "inherit" });
  } catch (err) {
    process.stderr.write(
      `[kandev]   (could not run tail: ${err instanceof Error ? err.message : String(err)})\n`,
    );
  }
}
