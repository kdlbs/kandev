import os from "node:os";
import path from "node:path";

// Default service ports (will auto-fallback if busy).
export const DEFAULT_BACKEND_PORT = 8080;
export const DEFAULT_WEB_PORT = 37429;
export const DEFAULT_AGENTCTL_PORT = 9999;
export const DEFAULT_MCP_PORT = 9090;

// Random fallback range for port selection.
export const RANDOM_PORT_MIN = 10000;
export const RANDOM_PORT_MAX = 60000;
export const RANDOM_PORT_RETRIES = 10;

// Backend healthcheck timeout during startup.
export const HEALTH_TIMEOUT_MS_RELEASE = 45000;
export const HEALTH_TIMEOUT_MS_DEV = 600000;

// Kandev root directory. Single source of truth for the dotdir name and
// everything derived from it (data, tasks, bin). Dev mode uses a separate
// root under the repo (see DEV_KANDEV_DOTDIR).
export const KANDEV_DOTDIR = ".kandev";
export const KANDEV_HOME_DIR = path.join(os.homedir(), KANDEV_DOTDIR);
export const KANDEV_TASKS_DIR = path.join(KANDEV_HOME_DIR, "tasks");

// Local user cache/data directories for release bundles and DB.
export const CACHE_DIR = path.join(KANDEV_HOME_DIR, "bin");
export const DATA_DIR = path.join(KANDEV_HOME_DIR, "data");

// Dev-mode root: an isolated kandev home inside the repo so that running
// `make dev` from inside a kandev-spawned task workspace does not touch the
// user's production state.
export const DEV_KANDEV_DOTDIR = ".kandev-dev";
export function devKandevHome(repoRoot: string): string {
  return path.join(repoRoot, DEV_KANDEV_DOTDIR);
}
