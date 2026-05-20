import { execFileSync, spawnSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";

import type { ServiceArgs } from "./args";
import { dumpJournalctlLogs, waitForServiceHealth } from "./health_check";
import { commandExists, writeUnitFile } from "./install_helpers";
import {
  captureLauncher,
  currentUsername,
  linuxUserUnitDir,
  LINUX_SYSTEM_UNIT_DIR,
  resolveHomeDir,
  resolveLogDir,
  resolveServiceUser,
  SERVICE_NAME,
} from "./paths";
import { renderSystemdUnit } from "./templates";

type Ctx = {
  args: ServiceArgs;
  systemctlArgs: string[];
  unitPath: string;
  isSystem: boolean;
};

function makeCtx(args: ServiceArgs): Ctx {
  const isSystem = !!args.system;
  const unitDir = isSystem ? LINUX_SYSTEM_UNIT_DIR : linuxUserUnitDir();
  const systemctlArgs = isSystem ? [] : ["--user"];
  return {
    args,
    systemctlArgs,
    unitPath: path.join(unitDir, `${SERVICE_NAME}.service`),
    isSystem,
  };
}

export async function runLinuxService(args: ServiceArgs): Promise<void> {
  if (!commandExists("systemctl")) {
    throw new Error("systemctl not found. Linux service install requires systemd.");
  }
  const ctx = makeCtx(args);
  switch (args.action) {
    case "install":
      return installAsync(ctx);
    case "uninstall":
      return uninstall(ctx);
    case "start":
      return runSystemctl(ctx, ["start", SERVICE_NAME]);
    case "stop":
      return runSystemctl(ctx, ["stop", SERVICE_NAME]);
    case "restart":
      return runSystemctl(ctx, ["restart", SERVICE_NAME]);
    case "status":
      return runSystemctl(ctx, ["status", SERVICE_NAME], { allowFailure: true });
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
  installSync(ctx);
  await waitForServiceHealth(ctx.args.port, () =>
    dumpJournalctlLogs({ unit: SERVICE_NAME, isSystem: ctx.isSystem, lines: 50 }),
  );
}

function installSync(ctx: Ctx): void {
  const launcher = captureLauncher();
  const homeDir = resolveHomeDir(ctx.args.homeDir, ctx.isSystem);
  const logDir = resolveLogDir(homeDir);
  const unit = renderSystemdUnit({
    launcher,
    homeDir,
    logDir,
    port: ctx.args.port,
    systemUser: ctx.isSystem ? resolveServiceUser(true) : undefined,
    mode: ctx.isSystem ? "system" : "user",
  });

  fs.mkdirSync(path.dirname(ctx.unitPath), { recursive: true });
  const outcome = writeUnitFile(ctx.unitPath, unit);

  runSystemctl(ctx, ["daemon-reload"]);
  // Always run enable --now so 'install' is fully idempotent: if the user
  // manually disabled or stopped the service, re-running install brings it
  // back online without changing the unit file.
  runSystemctl(ctx, ["enable", "--now", SERVICE_NAME]);
  console.log(
    outcome === "unchanged"
      ? "[kandev] service is enabled and running"
      : "[kandev] service enabled and started",
  );

  if (!ctx.isSystem && !ctx.args.noBootStart) {
    maybePromptLinger();
  }

  printPostInstallHints(ctx);
}

function uninstall(ctx: Ctx): void {
  // Disable and stop, ignoring failures since the unit may already be stopped.
  runSystemctl(ctx, ["disable", "--now", SERVICE_NAME], { allowFailure: true });
  if (fs.existsSync(ctx.unitPath)) {
    fs.unlinkSync(ctx.unitPath);
    console.log(`[kandev] removed ${ctx.unitPath}`);
  } else {
    console.log(`[kandev] no unit file at ${ctx.unitPath}`);
  }
  runSystemctl(ctx, ["daemon-reload"], { allowFailure: true });
}

function showLogs(ctx: Ctx): void {
  const journalArgs: string[] = ctx.isSystem ? ["-u", SERVICE_NAME] : ["--user-unit", SERVICE_NAME];
  if (ctx.args.follow) journalArgs.push("-f");
  else journalArgs.push("-n", "200", "--no-pager");

  const res = spawnSync("journalctl", journalArgs, { stdio: "inherit" });
  if (res.status !== 0 && !ctx.args.follow) {
    throw new Error(`journalctl exited with code ${res.status}`);
  }
}

function runSystemctl(ctx: Ctx, args: string[], opts: { allowFailure?: boolean } = {}): void {
  const argv = [...ctx.systemctlArgs, ...args];
  const res = spawnSync("systemctl", argv, { stdio: "inherit" });
  if (res.status !== 0 && !opts.allowFailure) {
    throw new Error(`systemctl ${argv.join(" ")} failed with code ${res.status}`);
  }
}

function lingerEnabled(user: string): boolean {
  try {
    const out = execFileSync("loginctl", ["show-user", user, "--property=Linger"], {
      encoding: "utf8",
    });
    return out.trim().toLowerCase() === "linger=yes";
  } catch {
    // loginctl may not be available or user record missing — assume off.
    return false;
  }
}

function maybePromptLinger(): void {
  const user = currentUsername();
  if (lingerEnabled(user)) {
    console.log("[kandev] enable-linger already active — kandev will start at boot");
    return;
  }
  console.log("");
  console.log("[kandev] User services only run while you're logged in.");
  console.log("[kandev] To start kandev at boot, run:");
  console.log(`[kandev]   sudo loginctl enable-linger ${user}`);
  console.log("[kandev] (Pass --no-boot-start to skip this notice next time.)");
}

function printPostInstallHints(ctx: Ctx): void {
  const ctl = ctx.isSystem ? "sudo systemctl" : "systemctl --user";
  const journal = ctx.isSystem ? "sudo journalctl" : "journalctl --user-unit";
  console.log("");
  console.log("[kandev] Useful commands:");
  console.log(`[kandev]   ${ctl} status ${SERVICE_NAME}`);
  console.log(`[kandev]   ${ctl} restart ${SERVICE_NAME}`);
  console.log(`[kandev]   ${journal} ${ctx.isSystem ? "-u " : ""}${SERVICE_NAME} -f`);
}
