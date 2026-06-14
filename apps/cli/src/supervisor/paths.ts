import fs from "node:fs";
import path from "node:path";

import { resolveKandevHomeDir } from "../constants";

export function supervisorDir(homeDir = resolveKandevHomeDir()): string {
  return path.join(homeDir, "supervisor");
}

export function manifestPath(homeDir = resolveKandevHomeDir()): string {
  return path.join(supervisorDir(homeDir), "launch.json");
}

export function socketPath(homeDir = resolveKandevHomeDir()): string {
  if (process.platform === "win32") {
    const safe = homeDir.replace(/[^a-zA-Z0-9_.-]/g, "-");
    return `\\\\.\\pipe\\kandev-${safe}-supervisor`;
  }
  return path.join(supervisorDir(homeDir), "control.sock");
}

export function prepareSupervisorDir(homeDir = resolveKandevHomeDir()): string {
  const dir = supervisorDir(homeDir);
  fs.mkdirSync(dir, { recursive: true, mode: 0o700 });
  if (process.platform !== "win32") {
    fs.chmodSync(dir, 0o700);
  }
  return dir;
}
