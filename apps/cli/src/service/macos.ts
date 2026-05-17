import { execFileSync, spawnSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

import type { ServiceArgs } from "./args";
import { dumpLaunchdLogs, waitForServiceHealth } from "./health_check";
import { writeUnitFile } from "./install_helpers";
import { captureLauncher, resolveHomeDir, resolveLogDir, resolveServiceUser } from "./paths";
import { renderLaunchdPlist } from "./templates";

const LABEL = "com.kdlbs.kandev";
const USER_AGENT_DIR = path.join(os.homedir(), "Library", "LaunchAgents");
const SYSTEM_DAEMON_DIR = "/Library/LaunchDaemons";

type Ctx = {
  args: ServiceArgs;
  plistPath: string;
  isSystem: boolean;
  /** launchctl domain target, e.g. gui/501 or system. */
  domain: string;
};

function makeCtx(args: ServiceArgs): Ctx {
  const isSystem = !!args.system;
  const dir = isSystem ? SYSTEM_DAEMON_DIR : USER_AGENT_DIR;
  const uid = os.userInfo().uid;
  return {
    args,
    plistPath: path.join(dir, `${LABEL}.plist`),
    isSystem,
    domain: isSystem ? "system" : `gui/${uid}`,
  };
}

export async function runMacosService(args: ServiceArgs): Promise<void> {
  if (!commandExists("launchctl")) {
    throw new Error("launchctl not found. macOS service install requires launchd.");
  }
  const ctx = makeCtx(args);
  switch (args.action) {
    case "install":
      return installAsync(ctx);
    case "uninstall":
      return uninstall(ctx);
    case "start":
      return startService(ctx);
    case "stop":
      return stopService(ctx);
    case "restart":
      stopService(ctx);
      return startService(ctx);
    case "status":
      return showStatus(ctx);
    case "logs":
      return showLogs(ctx);
  }
}

async function installAsync(ctx: Ctx): Promise<void> {
  installSync(ctx);
  const homeDir = resolveHomeDir(ctx.args.homeDir, ctx.isSystem);
  const logDir = resolveLogDir(homeDir);
  await waitForServiceHealth(ctx.args.port, () => dumpLaunchdLogs({ logDir, lines: 50 }));
}

function installSync(ctx: Ctx): void {
  const launcher = captureLauncher();
  const homeDir = resolveHomeDir(ctx.args.homeDir, ctx.isSystem);
  const logDir = resolveLogDir(homeDir);
  fs.mkdirSync(logDir, { recursive: true });

  const plist = renderLaunchdPlist({
    launcher,
    homeDir,
    logDir,
    port: ctx.args.port,
    systemUser: ctx.isSystem ? resolveServiceUser(true) : undefined,
    mode: ctx.isSystem ? "system" : "user",
  });

  fs.mkdirSync(path.dirname(ctx.plistPath), { recursive: true });
  const outcome = writeUnitFile(ctx.plistPath, plist);

  // launchctl bootstrap fails if the label is already loaded — bootout first
  // (ignoring its error if nothing was loaded). This means 'install' is
  // idempotent: re-running it reloads the unit even if the file is unchanged,
  // which is how we recover from a user manually unloading the service.
  spawnSync("launchctl", ["bootout", `${ctx.domain}/${LABEL}`], { stdio: "ignore" });
  runLaunchctl(["bootstrap", ctx.domain, ctx.plistPath]);
  runLaunchctl(["enable", `${ctx.domain}/${LABEL}`], { allowFailure: true });
  console.log(
    outcome === "unchanged"
      ? "[kandev] service is loaded and running"
      : "[kandev] service loaded and started",
  );

  printPostInstallHints(ctx, logDir);
}

function uninstall(ctx: Ctx): void {
  runLaunchctl(["bootout", `${ctx.domain}/${LABEL}`], { allowFailure: true });
  if (fs.existsSync(ctx.plistPath)) {
    fs.unlinkSync(ctx.plistPath);
    console.log(`[kandev] removed ${ctx.plistPath}`);
  } else {
    console.log(`[kandev] no plist at ${ctx.plistPath}`);
  }
}

function startService(ctx: Ctx): void {
  runLaunchctl(["kickstart", `${ctx.domain}/${LABEL}`]);
}

function stopService(ctx: Ctx): void {
  runLaunchctl(["kill", "SIGTERM", `${ctx.domain}/${LABEL}`], { allowFailure: true });
}

function showStatus(ctx: Ctx): void {
  const res = spawnSync("launchctl", ["print", `${ctx.domain}/${LABEL}`], { stdio: "inherit" });
  if (res.status !== 0) {
    console.log(`[kandev] service not loaded in ${ctx.domain}`);
  }
}

function showLogs(ctx: Ctx): void {
  const homeDir = resolveHomeDir(ctx.args.homeDir, ctx.isSystem);
  const logDir = resolveLogDir(homeDir);
  const outPath = path.join(logDir, "service.out");
  const errPath = path.join(logDir, "service.err");
  const tailArgs: string[] = ctx.args.follow ? ["-f", "-n", "200"] : ["-n", "200"];
  const targets = [outPath, errPath].filter((p) => fs.existsSync(p));
  if (targets.length === 0) {
    console.log(`[kandev] no logs yet at ${logDir}`);
    return;
  }
  spawnSync("tail", [...tailArgs, ...targets], { stdio: "inherit" });
}

function runLaunchctl(args: string[], opts: { allowFailure?: boolean } = {}): void {
  const res = spawnSync("launchctl", args, { stdio: "inherit" });
  if (res.status !== 0 && !opts.allowFailure) {
    throw new Error(`launchctl ${args.join(" ")} failed with code ${res.status}`);
  }
}

function commandExists(cmd: string): boolean {
  try {
    execFileSync("which", [cmd], { stdio: "ignore" });
    return true;
  } catch {
    return false;
  }
}

function printPostInstallHints(ctx: Ctx, logDir: string): void {
  console.log("");
  console.log("[kandev] Useful commands:");
  console.log(`[kandev]   launchctl print ${ctx.domain}/${LABEL}`);
  console.log(`[kandev]   kandev service restart${ctx.isSystem ? " --system" : ""}`);
  console.log(`[kandev]   tail -f ${path.join(logDir, "service.err")}`);
}
