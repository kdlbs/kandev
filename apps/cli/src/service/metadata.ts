import fs from "node:fs";
import path from "node:path";

import type { LauncherInfo } from "./paths";

export type ServiceManager = "systemd" | "launchd";
export type ServiceMode = "user" | "system";

export type ServiceInstallMetadata = {
  version: 1;
  manager: ServiceManager;
  mode: ServiceMode;
  kind: LauncherInfo["kind"];
  home_dir: string;
  log_dir: string;
  service_path: string;
  node_path: string;
  cli_entry: string;
  bundle_dir?: string;
  launcher_version?: string;
  port?: number;
  system_user?: string;
  installed_at: string;
};

export type BuildServiceInstallMetadataInput = {
  manager: ServiceManager;
  mode: ServiceMode;
  launcher: LauncherInfo;
  homeDir: string;
  logDir: string;
  servicePath: string;
  port?: number;
  systemUser?: string;
  now?: Date;
};

export function serviceMetadataPath(homeDir: string): string {
  return path.join(homeDir, "service", "install.json");
}

export function buildServiceInstallMetadata(
  input: BuildServiceInstallMetadataInput,
): ServiceInstallMetadata {
  const out: ServiceInstallMetadata = {
    version: 1,
    manager: input.manager,
    mode: input.mode,
    kind: input.launcher.kind,
    home_dir: input.homeDir,
    log_dir: input.logDir,
    service_path: input.servicePath,
    node_path: input.launcher.nodePath,
    cli_entry: input.launcher.cliEntry,
    installed_at: (input.now ?? new Date()).toISOString(),
  };
  if (input.launcher.bundleDir) out.bundle_dir = input.launcher.bundleDir;
  if (input.launcher.version) out.launcher_version = input.launcher.version;
  if (input.port !== undefined) out.port = input.port;
  if (input.systemUser) out.system_user = input.systemUser;
  return out;
}

export function writeServiceInstallMetadata(
  metadataPath: string,
  metadata: ServiceInstallMetadata,
): void {
  fs.mkdirSync(path.dirname(metadataPath), { recursive: true, mode: 0o700 });
  fs.writeFileSync(metadataPath, `${JSON.stringify(metadata, null, 2)}\n`, { mode: 0o600 });
}
