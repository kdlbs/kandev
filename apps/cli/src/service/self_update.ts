import { spawnSync, type SpawnSyncReturns } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

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

export type CommandResult = Pick<SpawnSyncReturns<Buffer>, "status" | "error"> & {
  stdout?: Buffer | string | null;
  stderr?: Buffer | string | null;
};

export type CommandRunner = (command: string, args: string[]) => CommandResult;

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
    // `brew upgrade` installs the tap formula's current version; it can't be
    // pinned to intent.target_version the way npm/npx can (Homebrew has no
    // stable "install version X" without a separate versioned formula). In
    // practice both target_version and the formula derive from the same latest
    // GitHub release, so they match. If the formula lags, the restarted backend
    // reports the older version and the frontend progress poll (which waits for
    // info.version === target_version) times out gracefully rather than
    // reporting a false success.
    commands.push({ command: "brew", args: ["upgrade", "kandev"] });
    // Re-run service install via the upgraded `kandev` wrapper (resolved on
    // PATH), NOT `node <cli_entry>`. Homebrew installs into version-pinned
    // Cellar dirs, so the recorded cli_entry still points at the OLD bundle
    // after `brew upgrade`; worse, invoking node on it directly runs without
    // KANDEV_BUNDLE_DIR/KANDEV_VERSION, so the install is mis-detected as
    // kind "unknown" and the regenerated unit loses the bundle env — the
    // restarted backend then can't find its runtime and the service stays down.
    // The wrapper is version-stable and re-sets the bundle env. (Mirrors the
    // manual `kandev service install` we already document for Homebrew.)
    commands.push({ command: "kandev", args: installArgs });
  } else if (install.kind === "npm") {
    commands.push({ command: "npm", args: npmInstallArgs(install.cli_entry, target) });
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
  const log = openSelfUpdateLog(intent);
  try {
    log?.line(`self-update target ${intent.target_tag} (${intent.target_version})`);
    log?.line(`install kind=${intent.install.kind} manager=${intent.install.manager}`);
    log?.line(`planned ${commands.length} command(s):`);
    commands.forEach((step, i) =>
      log?.line(`  [${i + 1}/${commands.length}] ${formatCommand(step)}`),
    );
    runSelfUpdateSteps(commands, runner, log);
    log?.line("self-update completed successfully");
  } finally {
    log?.close();
  }
}

function runSelfUpdateSteps(
  commands: PlannedCommand[],
  runner: CommandRunner,
  log: SelfUpdateLog | null,
): void {
  for (const [i, step] of commands.entries()) {
    log?.line(`\n[${i + 1}/${commands.length}] $ ${formatCommand(step)}`);
    const res = runner(step.command, step.args);
    teeCommandOutput(log, res);
    if (res.error) {
      log?.line(`! spawn error: ${res.error.message}`);
      throw res.error;
    }
    if (res.status !== 0) {
      log?.line(`! exited with code ${res.status}`);
      throw new Error(`${formatCommand(step)} failed with code ${res.status}`);
    }
    log?.line("  exit 0");
  }
}

function formatCommand(step: PlannedCommand): string {
  return [step.command, ...step.args].join(" ");
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

function npmInstallArgs(cliEntry: string, target: string): string[] {
  const args = ["install", "-g"];
  const prefix = npmPrefixFromCliEntry(cliEntry);
  if (prefix) {
    args.push("--prefix", prefix);
  }
  args.push(`kandev@${target}`);
  return args;
}

function npmPrefixFromCliEntry(cliEntry: string): string | undefined {
  const marker = `${path.sep}lib${path.sep}node_modules${path.sep}kandev${path.sep}`;
  const index = cliEntry.indexOf(marker);
  if (index < 0) {
    return undefined;
  }
  return index === 0 ? path.sep : cliEntry.slice(0, index);
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

function spawnCommand(command: string, args: string[]): CommandResult {
  // Capture stdout/stderr (rather than inheriting) so the self-update log can
  // tee each step's output. The helper runs detached under launchd/systemd, so
  // its console output would otherwise go nowhere visible — the log file is the
  // only forensic trail when an update fails mid-flight. maxBuffer is bumped
  // well above the 1 MiB default because `npm install -g` is chatty.
  return spawnSync(command, args, { maxBuffer: 64 * 1024 * 1024 });
}

type SelfUpdateLog = {
  path: string;
  line: (message: string) => void;
  raw: (chunk: Buffer | string) => void;
  close: () => void;
};

/**
 * Open a timestamped log file under the service's log dir for this self-update
 * run. The helper is spawned outside the service process (so a restart doesn't
 * kill it mid-update), which means it has no service log of its own — without
 * this file a failed update on macOS/launchd is nearly invisible. Best-effort:
 * if the log dir can't be created we return null and the update still runs.
 */
function openSelfUpdateLog(intent: SelfUpdateIntent): SelfUpdateLog | null {
  const dir = intent.install.log_dir || logDirFromHome(intent.install.home_dir);
  if (!dir) {
    return null;
  }
  try {
    fs.mkdirSync(dir, { recursive: true });
    const stamp = new Date().toISOString().replace(/[:.]/g, "-");
    const filePath = path.join(dir, `self-update-${stamp}.log`);
    const fd = fs.openSync(filePath, "a");
    const write = (chunk: Buffer | string): void => {
      try {
        // Branch so TypeScript can resolve the writeSync overload — a
        // `Buffer | string` union matches neither overload directly.
        if (typeof chunk === "string") {
          fs.writeSync(fd, chunk);
        } else {
          fs.writeSync(fd, chunk);
        }
      } catch {
        // A write failure mid-update must not abort the update itself.
      }
    };
    process.stdout.write(`[kandev] self-update log: ${filePath}\n`);
    write(`# kandev self-update ${new Date().toISOString()}\n`);
    return {
      path: filePath,
      line: (message) => write(`${message}\n`),
      raw: (chunk) => write(chunk),
      close: () => {
        try {
          fs.closeSync(fd);
        } catch {
          // already closed / never opened
        }
      },
    };
  } catch {
    return null;
  }
}

function logDirFromHome(homeDir: string): string {
  return homeDir ? path.join(homeDir, "logs") : "";
}

// Mirror a command's captured output to both the helper log and the process's
// own stdout/stderr, so whatever does manage to capture the helper's streams
// (e.g. launchd's StandardOut/ErrorPath, systemd's journal) still sees it.
function teeCommandOutput(log: SelfUpdateLog | null, res: CommandResult): void {
  if (res.stdout && res.stdout.length) {
    process.stdout.write(res.stdout);
    log?.raw(res.stdout);
  }
  if (res.stderr && res.stderr.length) {
    process.stderr.write(res.stderr);
    log?.raw(res.stderr);
  }
}
