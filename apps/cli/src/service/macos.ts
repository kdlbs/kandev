import { spawnSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

import type { ServiceArgs } from "./args";
import { dumpLaunchdLogs, waitForServiceHealth } from "./health_check";
import { commandExists, writeUnitFile } from "./install_helpers";
import {
  captureLauncher,
  LAUNCHD_LABEL,
  macosUserAgentDir,
  MACOS_SYSTEM_DAEMON_DIR,
  resolveHomeDir,
  resolveLogDir,
  resolveServiceUser,
} from "./paths";
import { renderLaunchdPlist } from "./templates";

type Ctx = {
  args: ServiceArgs;
  plistPath: string;
  isSystem: boolean;
  /** launchctl domain target, e.g. gui/501 or system. */
  domain: string;
  /** Full launchctl service target, e.g. gui/501/com.kdlbs.kandev. */
  target: string;
};

function makeCtx(args: ServiceArgs): Ctx {
  const isSystem = !!args.system;
  const dir = isSystem ? MACOS_SYSTEM_DAEMON_DIR : macosUserAgentDir();
  const uid = os.userInfo().uid;
  const domain = isSystem ? "system" : `gui/${uid}`;
  return {
    args,
    plistPath: path.join(dir, `${LAUNCHD_LABEL}.plist`),
    isSystem,
    domain,
    target: `${domain}/${LAUNCHD_LABEL}`,
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
      return restartService(ctx);
    case "status":
      return showStatus(ctx);
    case "logs":
      return showLogs(ctx);
    case "config":
      // Handled by the dispatcher in index.ts before reaching the platform layer.
      throw new Error("unreachable: config action handled in service/index.ts");
    default: {
      const _exhaustive: never = args.action;
      throw new Error(`unhandled service action: ${_exhaustive as string}`);
    }
  }
}

async function installAsync(ctx: Ctx): Promise<void> {
  const { logDir } = installSync(ctx);
  await waitForServiceHealth(ctx.args.port, () => dumpLaunchdLogs({ logDir, lines: 50 }));
}

function installSync(ctx: Ctx): { logDir: string } {
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
  spawnSync("launchctl", ["bootout", ctx.target], { stdio: "ignore" });
  runLaunchctl(["bootstrap", ctx.domain, ctx.plistPath]);
  runLaunchctl(["enable", ctx.target], { allowFailure: true });
  console.log(
    outcome === "unchanged"
      ? "[kandev] service is loaded and running"
      : "[kandev] service loaded and started",
  );

  printPostInstallHints(ctx, logDir);
  return { logDir };
}

function uninstall(ctx: Ctx): void {
  runLaunchctl(["bootout", ctx.target], { allowFailure: true });
  if (fs.existsSync(ctx.plistPath)) {
    fs.unlinkSync(ctx.plistPath);
    console.log(`[kandev] removed ${ctx.plistPath}`);
  } else {
    console.log(`[kandev] no plist at ${ctx.plistPath}`);
  }
}

// `bootstrap` loads the job (start) and `bootout` fully unloads it (stop).
// We use these instead of `kickstart` + `kill` because the plist sets
// `KeepAlive=true` — under KeepAlive, `kill SIGTERM` does not stop the
// service: launchd just respawns it seconds later. Only `bootout` removes
// the job from launchd's supervision.
//
// start/restart both have to handle two pre-states — job loaded vs not —
// so each begins with a bootout-then-bootstrap dance similar to installSync.
function startService(ctx: Ctx): void {
  // Idempotent: if the label is already loaded, bootstrap would fail. Bootout
  // first so start works whether the service was previously running or stopped.
  spawnSync("launchctl", ["bootout", ctx.target], { stdio: "ignore" });
  runLaunchctl(["bootstrap", ctx.domain, ctx.plistPath]);
}

function stopService(ctx: Ctx): void {
  runLaunchctl(["bootout", ctx.target], { allowFailure: true });
}

// `kickstart -k` atomically kills and restarts a loaded service. If the job
// was previously stopped (bootout'd), the target no longer exists in the
// launchd domain and kickstart fails — fall back to bootstrap to reload it.
function restartService(ctx: Ctx): void {
  const res = spawnSync("launchctl", ["kickstart", "-k", ctx.target], { stdio: "inherit" });
  if (res.status === 0) return;
  runLaunchctl(["bootstrap", ctx.domain, ctx.plistPath]);
}

function showStatus(ctx: Ctx): void {
  const res = spawnSync("launchctl", ["print", ctx.target], {
    stdio: "inherit",
  });
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

function printPostInstallHints(ctx: Ctx, logDir: string): void {
  console.log("");
  console.log("[kandev] Useful commands:");
  console.log(`[kandev]   launchctl print ${ctx.target}`);
  console.log(`[kandev]   kandev service restart${ctx.isSystem ? " --system" : ""}`);
  console.log(`[kandev]   tail -f ${path.join(logDir, "service.err")}`);
}
