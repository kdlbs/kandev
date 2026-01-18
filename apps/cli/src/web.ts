import { spawn } from "node:child_process";

import { createProcessSupervisor } from "./process";

export type WebLaunchOptions = {
  command: string;
  args: string[];
  cwd: string;
  env: NodeJS.ProcessEnv;
  url: string;
  supervisor: ReturnType<typeof createProcessSupervisor>;
  label: string;
};

export function openBrowser(url: string) {
  if (process.env.KANDEV_NO_BROWSER === "1") {
    return;
  }
  const opener =
    process.platform === "darwin"
      ? "open"
      : process.platform === "win32"
        ? "cmd"
        : "xdg-open";
  const args =
    process.platform === "win32"
      ? ["/c", "start", "", url]
      : [url];
  try {
    const child = spawn(opener, args, { stdio: "ignore", detached: true });
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
  url,
  supervisor,
  label,
}: WebLaunchOptions) {
  const proc = spawn(command, args, { cwd, env, stdio: "inherit" });
  supervisor.children.push(proc);

  proc.on("exit", (code, signal) => {
    console.error(`[kandev] ${label} exited (code=${code}, signal=${signal})`);
    const exitCode = signal ? 0 : code ?? 1;
    void supervisor.shutdown(`${label} exit`).then(() => process.exit(exitCode));
  });

  openBrowser(url);
  return proc;
}
