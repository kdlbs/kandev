import os from "node:os";
import path from "node:path";

// Default service ports (will auto-fallback if busy).
export const DEFAULT_BACKEND_PORT = 8080;
export const DEFAULT_WEB_PORT = 3000;
export const DEFAULT_AGENTCTL_PORT = 9999;

// Random fallback range for port selection.
export const RANDOM_PORT_MIN = 10000;
export const RANDOM_PORT_MAX = 60000;
export const RANDOM_PORT_RETRIES = 10;

// Backend healthcheck timeout during startup.
export const HEALTH_TIMEOUT_MS = 15000;

// Local user cache/data directories for release bundles and DB.
export const CACHE_DIR = path.join(os.homedir(), ".kandev", "bin");
export const DATA_DIR = path.join(os.homedir(), ".kandev", "data");
