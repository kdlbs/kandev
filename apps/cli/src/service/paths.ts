import fs from "node:fs";
import os from "node:os";
import path from "node:path";

import { KANDEV_HOME_DIR } from "../constants";

export type LauncherKind = "homebrew" | "npm" | "unknown";

export type LauncherInfo = {
  /** Absolute path to node executable (process.execPath at install time). */
  nodePath: string;
  /** Absolute path to the cli.js entry point. */
  cliEntry: string;
  /** Best-guess of how kandev was installed. Used to seed env vars. */
  kind: LauncherKind;
  /** KANDEV_BUNDLE_DIR if set (Homebrew sets this). */
  bundleDir?: string;
  /** KANDEV_VERSION if set (Homebrew sets this). */
  version?: string;
};

/**
 * Snapshot the current invocation so the service unit can faithfully reproduce it.
 *
 * The unit file hard-codes absolute paths because systemd/launchd start with an
 * empty PATH and may not see the user's `node` or `kandev` shim. By recording
 * `process.execPath` (node) and `process.argv[1]` (cli.js) at install time we
 * avoid any PATH lookups at service-run time.
 */
export function captureLauncher(): LauncherInfo {
  const nodePath = process.execPath;
  const cliEntry = resolveCliEntry();
  const bundleDir = process.env.KANDEV_BUNDLE_DIR;
  const version = process.env.KANDEV_VERSION;
  const kind: LauncherKind = bundleDir
    ? "homebrew"
    : cliEntry.includes(`${path.sep}node_modules${path.sep}`)
      ? "npm"
      : "unknown";
  return { nodePath, cliEntry, kind, bundleDir, version };
}

function resolveCliEntry(): string {
  const argvEntry = process.argv[1];
  if (argvEntry && fs.existsSync(argvEntry)) {
    return path.resolve(argvEntry);
  }
  throw new Error(
    "could not resolve the kandev CLI entry path from process.argv[1]; " +
      "rerun via the kandev binary",
  );
}

/** Resolve the home directory used for the unit's KANDEV_HOME_DIR env. */
export function resolveHomeDir(override: string | undefined, runAsRoot: boolean): string {
  if (override) return path.resolve(override);
  if (runAsRoot) {
    // System units default to /var/lib/kandev so root-owned data lives outside any
    // single user's $HOME (where it would be unreachable to other users).
    return "/var/lib/kandev";
  }
  return KANDEV_HOME_DIR;
}

/** Absolute path to the log directory used by the unit for stdout/stderr. */
export function resolveLogDir(homeDir: string): string {
  return path.join(homeDir, "logs");
}

/** Current username (the EUID the CLI is running as). */
export function currentUsername(): string {
  return os.userInfo().username;
}

/**
 * Resolve which user the service should run as.
 *
 * For user-mode installs this is always the current user (matters for hints
 * printed back to the user, not for the unit itself — user units don't set
 * `User=`).
 *
 * For system-mode installs the CLI is typically invoked via sudo, which makes
 * `os.userInfo().username` resolve to `root`. We prefer `SUDO_USER` so the
 * daemon runs as the human who installed it (with access to their `~/.kandev`,
 * git config, agent CLI credentials, etc) rather than as root.
 *
 * If the user genuinely wants a root-owned daemon they can run sudo with
 * `-E` stripped or pass `--run-as root` (future flag) — but the common case
 * (`sudo kandev service install --system`) gets the safe default.
 */
export function resolveServiceUser(isSystem: boolean): string {
  if (!isSystem) {
    return currentUsername();
  }
  const sudoUser = process.env.SUDO_USER;
  if (sudoUser && sudoUser !== "root") {
    return sudoUser;
  }
  return currentUsername();
}
