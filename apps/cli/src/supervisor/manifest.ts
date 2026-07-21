import fs from "node:fs";
import path from "node:path";

import { manifestPath, prepareSupervisorDir } from "./paths";

export type LaunchManifest = {
  version: 1;
  backend_executable: string;
  argv: string[];
  cwd: string;
  env: Record<string, string>;
  home_dir: string;
  port: number;
  mode: string;
  created_at: string;
};

export type LaunchManifestInput = Omit<LaunchManifest, "version" | "created_at" | "env"> & {
  env: NodeJS.ProcessEnv;
  now?: Date;
};

const ENV_ALLOWLIST = new Set([
  "KANDEV_HOME_DIR",
  "KANDEV_DATABASE_PATH",
  "KANDEV_SERVER_PORT",
  "KANDEV_WEB_INTERNAL_URL",
  "KANDEV_AGENT_STANDALONE_PORT",
  "KANDEV_LOG_LEVEL",
  "KANDEV_DEBUG_DEV_MODE",
  "KANDEV_DEBUG_AGENT_MESSAGES",
  "KANDEV_DEBUG_PPROF_ENABLED",
  "KANDEV_E2E_MOCK",
  "KANDEV_MOCK_AGENT",
  "KANDEV_MOCK_GITHUB",
  "KANDEV_MOCK_JIRA",
  "KANDEV_MOCK_LINEAR",
  "KANDEV_SUPERVISOR_SOCKET",
  "KANDEV_SUPERVISOR_MANIFEST",
  "KANDEV_RESTART_ADAPTER",
]);

export function buildLaunchManifest(input: LaunchManifestInput): LaunchManifest {
  if (!path.isAbsolute(input.backend_executable)) {
    throw new Error("backend_executable must be absolute");
  }
  if (!path.isAbsolute(input.cwd)) {
    throw new Error("cwd must be absolute");
  }
  if (!path.isAbsolute(input.home_dir)) {
    throw new Error("home_dir must be absolute");
  }
  return {
    version: 1,
    backend_executable: input.backend_executable,
    argv: [...input.argv],
    cwd: input.cwd,
    env: allowedEnv(input.env),
    home_dir: input.home_dir,
    port: input.port,
    mode: input.mode,
    created_at: (input.now ?? new Date()).toISOString(),
  };
}

export function writeLaunchManifest(
  manifest: LaunchManifest,
  targetPath = manifestPath(manifest.home_dir),
): string {
  prepareSupervisorDir(manifest.home_dir);
  fs.writeFileSync(targetPath, `${JSON.stringify(manifest, null, 2)}\n`, { mode: 0o600 });
  if (process.platform !== "win32") {
    fs.chmodSync(targetPath, 0o600);
  }
  return targetPath;
}

export function readLaunchManifest(targetPath: string): LaunchManifest {
  return JSON.parse(fs.readFileSync(targetPath, "utf8")) as LaunchManifest;
}

export function allowedEnv(env: NodeJS.ProcessEnv): Record<string, string> {
  const out: Record<string, string> = {};
  for (const key of ENV_ALLOWLIST) {
    const value = env[key];
    if (value !== undefined) {
      out[key] = value;
    }
  }
  return out;
}
