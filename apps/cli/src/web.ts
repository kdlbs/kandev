import { spawn, type ChildProcess, type StdioOptions } from "node:child_process";
import { readFileSync } from "node:fs";

import { createProcessSupervisor } from "./process";

// Node CLIs that install as .cmd shims on Windows. Anything in this set gets
// ".cmd" appended on win32 so Node's spawn (which doesn't apply PATHEXT) can
// resolve them.
const WIN_SHIM_COMMANDS = new Set(["pnpm", "npm", "npx", "yarn"]);

function resolveWindowsShim(command: string): string {
  if (process.platform !== "win32") return command;
  if (!WIN_SHIM_COMMANDS.has(command)) return command;
  return `${command}.cmd`;
}

let _isWSL: boolean | undefined;
function isWSL(): boolean {
  if (_isWSL === undefined) {
    try {
      _isWSL = readFileSync("/proc/version", "utf8").toLowerCase().includes("microsoft");
    } catch {
      _isWSL = false;
    }
  }
  return _isWSL;
}

export type WebLaunchOptions = {
  command: string;
  args: string[];
  cwd: string;
  env: NodeJS.ProcessEnv;
  supervisor: ReturnType<typeof createProcessSupervisor>;
  label: string;
  /** Suppress stdout/stderr output */
  quiet?: boolean;
};

export function openBrowser(url: string) {
  if (process.env.KANDEV_NO_BROWSER === "1") {
    return;
  }
  const useCmd = process.platform === "win32" || isWSL();
  const opener = process.platform === "darwin" ? "open" : useCmd ? "cmd.exe" : "xdg-open";
  const args = useCmd ? ["/c", "start", "", url] : [url];
  try {
    const child = spawn(opener, args, { stdio: "ignore", detached: true });
    child.on("error", () => {}); // ignore async spawn errors (e.g. xdg-open missing)
    child.unref();
  } catch {
    // ignore browser launch errors
  }
}

export function launchWebApp({
  command,
  args,
  cwd,
  env,
  supervisor,
  label,
  quiet = false,
}: WebLaunchOptions): ChildProcess {
  const stdio: StdioOptions = quiet ? ["ignore", "pipe", "pipe"] : "inherit";
  // Node's spawn doesn't apply PATHEXT on Windows: passing "pnpm" looks for a
  // literal file named "pnpm", but pnpm installs as "pnpm.cmd" (a batch shim).
  // Same goes for npm/npx/yarn. We resolve the .cmd extension explicitly
  // instead of using shell:true — shell:true with args trips Node 24's
  // DEP0190 security warning (args aren't escaped, only concatenated) and
  // forks a cmd.exe per spawn for no real benefit here.
  const resolved = resolveWindowsShim(command);
  const proc = spawn(resolved, args, { cwd, env, stdio });
  supervisor.children.push(proc);

  // In quiet mode, only forward stderr
  if (quiet && proc.stderr) {
    proc.stderr.pipe(process.stderr);
  }

  proc.on("exit", (code, signal) => {
    console.error(`[kandev] ${label} exited (code=${code}, signal=${signal})`);
    const exitCode = signal ? 0 : (code ?? 1);
    void supervisor.shutdown(`${label} exit`).then(() => process.exit(exitCode));
  });

  return proc;
}
