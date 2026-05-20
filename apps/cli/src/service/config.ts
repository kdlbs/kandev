import { execFileSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";

import { DEFAULT_BACKEND_PORT } from "../constants";
import type { ServiceArgs } from "./args";
import {
  captureLauncher,
  LAUNCHD_LABEL,
  linuxSystemUnitPath,
  linuxUserUnitPath,
  macosSystemPlistPath,
  macosUserPlistPath,
  resolveHomeDir,
  resolveLogDir,
  resolveServiceUser,
  SERVICE_NAME,
} from "./paths";
import { looksLikeManagedUnit } from "./templates";

/**
 * Print a human-readable summary of what kandev knows about the local
 * service install: paths, env vars, whether a unit exists, whether it's
 * active. Used for "why isn't it running?" diagnosis without needing to
 * remember the right systemctl / launchctl invocation.
 */
export function printServiceConfig(args: ServiceArgs): void {
  const isSystem = !!args.system;
  const launcher = captureLauncher();
  const homeDir = resolveHomeDir(args.homeDir, isSystem);
  const logDir = resolveLogDir(homeDir);

  console.log("=== kandev service config ===");
  console.log(`platform:        ${process.platform}`);
  console.log(`mode:            ${isSystem ? "system" : "user"}`);
  console.log(`launcher kind:   ${launcher.kind}`);
  console.log(`node path:       ${launcher.nodePath}`);
  console.log(`cli entry:       ${launcher.cliEntry}`);
  if (launcher.bundleDir) console.log(`bundle dir:      ${launcher.bundleDir}`);
  if (launcher.version) console.log(`version:         ${launcher.version}`);
  console.log("");
  console.log(`KANDEV_HOME_DIR: ${homeDir}`);
  console.log(`log dir:         ${logDir}`);
  console.log(`port:            ${args.port ?? `(default ${DEFAULT_BACKEND_PORT})`}`);
  if (isSystem) {
    console.log(`run as user:     ${resolveServiceUser(true)}`);
  }
  console.log("");

  if (process.platform === "linux") {
    printLinuxUnit(isSystem);
  } else if (process.platform === "darwin") {
    printMacosUnit(isSystem);
  } else {
    console.log(`unit:            (unsupported on ${process.platform})`);
  }
}

function printLinuxUnit(isSystem: boolean): void {
  const unitPath = isSystem ? linuxSystemUnitPath() : linuxUserUnitPath();
  console.log(`unit path:       ${unitPath}`);
  const present = fs.existsSync(unitPath);
  console.log(`installed:       ${present ? "yes" : "no"}`);
  if (present) {
    const content = safeRead(unitPath);
    console.log(`managed by us:   ${content && looksLikeManagedUnit(content) ? "yes" : "no"}`);
  }
  const active = systemctlIsActive(isSystem);
  if (active !== null) console.log(`active state:    ${active}`);
}

function printMacosUnit(isSystem: boolean): void {
  const plistPath = isSystem ? macosSystemPlistPath() : macosUserPlistPath();
  console.log(`plist path:      ${plistPath}`);
  const present = fs.existsSync(plistPath);
  console.log(`installed:       ${present ? "yes" : "no"}`);
  if (present) {
    const content = safeRead(plistPath);
    console.log(`managed by us:   ${content && looksLikeManagedUnit(content) ? "yes" : "no"}`);
  }
  console.log(`loaded:          ${launchctlIsLoaded(isSystem) ? "yes" : "no"}`);
}

function safeRead(p: string): string | null {
  try {
    return fs.readFileSync(p, "utf8");
  } catch {
    return null;
  }
}

function systemctlIsActive(isSystem: boolean): string | null {
  try {
    const args = isSystem ? ["is-active", SERVICE_NAME] : ["--user", "is-active", SERVICE_NAME];
    const out = execFileSync("systemctl", args, { encoding: "utf8" });
    return out.trim();
  } catch (err: unknown) {
    // is-active returns nonzero for inactive/failed/unknown — read the output anyway.
    // execFileSync attaches stdout to the thrown error; guard with a type check
    // instead of an unsafe cast so this keeps working if the error shape changes.
    if (err !== null && typeof err === "object" && "stdout" in err) {
      const { stdout } = err as { stdout?: Buffer | string };
      if (stdout) return String(stdout).trim();
    }
    return null;
  }
}

function launchctlIsLoaded(isSystem: boolean): boolean {
  try {
    const domain = isSystem ? "system" : `gui/${os.userInfo().uid}`;
    execFileSync("launchctl", ["print", `${domain}/${LAUNCHD_LABEL}`], { stdio: "ignore" });
    return true;
  } catch {
    return false;
  }
}
