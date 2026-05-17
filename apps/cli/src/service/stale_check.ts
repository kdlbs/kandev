import fs from "node:fs";

import { captureLauncher, linuxUserUnitPath, macosUserPlistPath } from "./paths";
import { looksLikeManagedUnit } from "./templates";

/**
 * Locations of the user-mode service unit we manage. We skip system-mode
 * locations because (a) they require sudo to fix anyway, (b) reading from
 * arbitrary `/etc` paths from a regular launch is noisier than worth it.
 */
function userModeUnitPath(): string | null {
  if (process.platform === "linux") return linuxUserUnitPath();
  if (process.platform === "darwin") return macosUserPlistPath();
  return null;
}

/**
 * Check whether an installed user-mode service unit still references the
 * current invocation's paths.
 *
 * Called once per interactive `kandev` start (skipped for `kandev service ...`
 * commands and headless service runs). The check is intentionally cheap and
 * silent on the happy path — only emits when a problem is detected — so it's
 * fine to run unconditionally.
 *
 * Returns the warning message to print, or null when there's nothing to say.
 */
export function detectStaleServiceUnit(): string | null {
  const unitPath = userModeUnitPath();
  if (!unitPath) return null;

  let content: string;
  try {
    content = fs.readFileSync(unitPath, "utf8");
  } catch {
    return null; // no unit installed
  }
  if (!looksLikeManagedUnit(content)) return null;

  let launcher: ReturnType<typeof captureLauncher>;
  try {
    launcher = captureLauncher();
  } catch {
    // captureLauncher throws when process.argv[1] is missing — extremely rare,
    // but if it happens we can't tell whether the unit is stale or not.
    return null;
  }
  // Stale = the unit's hard-coded paths no longer match the running binary.
  // We match on substring rather than parsing the unit so the check works for
  // both systemd Environment= lines and plist <string> entries.
  const nodeFresh = content.includes(launcher.nodePath);
  const cliFresh = content.includes(launcher.cliEntry);
  if (nodeFresh && cliFresh) return null;

  return (
    `Your installed kandev service unit (${unitPath}) references paths that\n` +
    `   no longer match this binary. This usually happens after upgrading via\n` +
    `   npm or Homebrew. Re-run 'kandev service install' to refresh the unit.`
  );
}

/** Convenience: detect + print to stderr in one call. Safe to call from cli.ts. */
export function warnIfStaleServiceUnit(): void {
  try {
    const msg = detectStaleServiceUnit();
    if (msg) {
      process.stderr.write(`[kandev] notice: ${msg}\n`);
    }
  } catch {
    // Never let a diagnostic check break startup.
  }
}
