import { spawn, type ChildProcess, type StdioOptions } from "node:child_process";
import { readFileSync } from "node:fs";

import { createProcessSupervisor } from "./process";

// Node CLIs that install as .cmd shims on Windows. Anything in this set goes
// through cmd.exe /c on win32 so we can spawn the shim safely:
//   - Node's spawn doesn't apply PATHEXT, so spawn("pnpm", …) hits ENOENT.
//   - spawn("pnpm.cmd", …) directly returns EINVAL on Node 21+ (the
//     CVE-2024-27980 fix forbids direct .bat/.cmd spawning).
//   - spawn(…, {shell:true}) works but trips DEP0190 (the args-aren't-escaped
//     security warning) and forks an extra cmd.exe per spawn.
// Going through cmd.exe explicitly is the cleanest option: no warning, no
// EINVAL, and the args (-C, --filter, identifier-like strings) contain no
// cmd metacharacters that need extra escaping.
const WIN_SHIM_COMMANDS = new Set(["pnpm", "npm", "npx", "yarn"]);

function resolveWindowsShim(command: string, args: string[]): { command: string; args: string[] } {
  if (process.platform !== "win32") return { command, args };
  if (!WIN_SHIM_COMMANDS.has(command)) return { command, args };
  return { command: "cmd.exe", args: ["/c", command, ...args] };
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
  const resolved = resolveWindowsShim(command, args);
  const proc = spawn(resolved.command, resolved.args, { cwd, env, stdio });
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
