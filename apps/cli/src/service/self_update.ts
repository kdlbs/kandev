import { spawnSync, type SpawnSyncReturns } from "node:child_process";
import fs from "node:fs";
import os from "node:os";

import type { ServiceArgs } from "./args";
import type { ServiceInstallMetadata } from "./metadata";
import { LAUNCHD_LABEL, SERVICE_NAME } from "./paths";

export type SelfUpdateIntent = {
  version: 1;
  target_tag: string;
  target_version: string;
  latest_url?: string;
  install: ServiceInstallMetadata;
  created_at: string;
};

export type PlannedCommand = {
  command: string;
  args: string[];
};

export type PlanSelfUpdateOptions = {
  platform?: NodeJS.Platform;
  uid?: number;
};

export type CommandRunner = (
  command: string,
  args: string[],
) => Pick<SpawnSyncReturns<Buffer>, "status" | "error">;

export function readSelfUpdateIntent(intentPath: string): SelfUpdateIntent {
  return JSON.parse(fs.readFileSync(intentPath, "utf8")) as SelfUpdateIntent;
}

export function planSelfUpdate(
  intent: SelfUpdateIntent,
  opts: PlanSelfUpdateOptions = {},
): PlannedCommand[] {
  const platform = opts.platform ?? process.platform;
  const target = npmVersion(intent.target_version || intent.target_tag);
  const install = intent.install;
  const installArgs = serviceInstallArgs(install);
  const commands: PlannedCommand[] = [];

  if (install.kind === "homebrew") {
    commands.push({ command: "brew", args: ["upgrade", "kandev"] });
    commands.push({ command: install.node_path, args: [install.cli_entry, ...installArgs] });
  } else if (install.kind === "npm") {
    commands.push({ command: "npm", args: ["install", "-g", `kandev@${target}`] });
    commands.push({ command: install.node_path, args: [install.cli_entry, ...installArgs] });
  } else if (install.kind === "npx") {
    commands.push({ command: "npx", args: ["-y", `kandev@${target}`, ...installArgs] });
  } else {
    throw new Error(`unsupported install kind "${install.kind}"`);
  }

  commands.push(restartCommand(install, platform, opts.uid));
  return commands;
}

export function runSelfUpdateCommand(
  args: ServiceArgs,
  runner: CommandRunner = spawnCommand,
): void {
  if (!args.intent) {
    throw new Error("kandev service self-update requires --intent <path>");
  }
  const intent = readSelfUpdateIntent(args.intent);
  const commands = planSelfUpdate(intent);
  if (args.dryRun || process.env.KANDEV_E2E_MOCK === "true") {
    console.log(
      JSON.stringify(
        {
          dry_run: !!args.dryRun,
          fake: process.env.KANDEV_E2E_MOCK === "true",
          target_version: intent.target_version,
          commands,
        },
        null,
        2,
      ),
    );
    return;
  }
  for (const step of commands) {
    const res = runner(step.command, step.args);
    if (res.error) {
      throw res.error;
    }
    if (res.status !== 0) {
      throw new Error(`${step.command} ${step.args.join(" ")} failed with code ${res.status}`);
    }
  }
}

function serviceInstallArgs(install: ServiceInstallMetadata): string[] {
  const args = ["service", "install"];
  if (install.mode === "system") args.push("--system");
  args.push("--home-dir", install.home_dir);
  if (install.port !== undefined) {
    args.push("--port", String(install.port));
  }
  return args;
}

function restartCommand(
  install: ServiceInstallMetadata,
  platform: NodeJS.Platform,
  uid = os.userInfo().uid,
): PlannedCommand {
  if (platform === "linux") {
    return install.mode === "system"
      ? { command: "systemctl", args: ["restart", SERVICE_NAME] }
      : { command: "systemctl", args: ["--user", "restart", SERVICE_NAME] };
  }
  if (platform === "darwin") {
    const domain = install.mode === "system" ? "system" : `gui/${uid}`;
    return { command: "launchctl", args: ["kickstart", "-k", `${domain}/${LAUNCHD_LABEL}`] };
  }
  throw new Error(`unsupported platform "${platform}"`);
}

function npmVersion(versionOrTag: string): string {
  const stripped = versionOrTag.replace(/^v/, "");
  return stripped || "latest";
}

function spawnCommand(
  command: string,
  args: string[],
): Pick<SpawnSyncReturns<Buffer>, "status" | "error"> {
  return spawnSync(command, args, { stdio: "inherit" });
}
