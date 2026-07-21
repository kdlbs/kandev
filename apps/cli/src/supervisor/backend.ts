import { spawn, spawnSync, type ChildProcess, type StdioOptions } from "node:child_process";
import path from "node:path";

import type { PortConfig } from "../shared";
import { attachBackendExitHandler } from "../shared";
import type { ChildLike, createProcessSupervisor } from "../process";
import { buildLaunchManifest, writeLaunchManifest } from "./manifest";
import { manifestPath, prepareSupervisorDir, socketPath } from "./paths";
import { createRestartableChild } from "./child";
import { startControlServer, type ControlServer } from "./control";

export type BackendSupervisorHandle = {
  proc: ChildProcess;
  control: ControlServer | null;
  env: NodeJS.ProcessEnv;
};

export async function launchRestartableBackend({
  command,
  args,
  cwd,
  env,
  homeDir,
  ports,
  mode,
  stdio,
  supervisor,
}: {
  command: string;
  args: string[];
  cwd: string;
  env: NodeJS.ProcessEnv;
  homeDir: string;
  ports: PortConfig;
  mode: string;
  stdio: StdioOptions;
  supervisor: ReturnType<typeof createProcessSupervisor>;
}): Promise<BackendSupervisorHandle> {
  if (!shouldUseSupervisor(env)) {
    const proc = spawn(command, args, { cwd, env, stdio });
    supervisor.children.push(proc as ChildLike);
    attachBackendExitHandler(proc, supervisor);
    return { proc, control: null, env };
  }

  const supervisorEnv = withSupervisorEnv(env, homeDir);
  const manifest = buildLaunchManifest({
    backend_executable: resolveExecutable(command),
    argv: args,
    cwd,
    env: supervisorEnv,
    home_dir: homeDir,
    port: ports.backendPort,
    mode,
  });
  writeLaunchManifest(manifest, supervisorEnv.KANDEV_SUPERVISOR_MANIFEST);

  const child = createRestartableChild(manifest, { stdio, extraEnv: supervisorEnv });
  let restarting = false;
  const attachExit = (proc: ChildProcess) => {
    attachBackendExitHandler(proc, supervisor, {
      shouldShutdown: () => !restarting,
    });
  };
  const proc = child.start();
  supervisor.children.push(proc as ChildLike);
  attachExit(proc);

  const control = await startControlServer(supervisorEnv.KANDEV_SUPERVISOR_SOCKET, async () => {
    restarting = true;
    try {
      const next = await child.restart();
      supervisor.children.push(next as ChildLike);
      attachExit(next);
    } finally {
      restarting = false;
    }
  });

  return { proc, control, env: supervisorEnv };
}

export function shouldUseSupervisor(env: NodeJS.ProcessEnv = process.env): boolean {
  return env.KANDEV_NO_SUPERVISOR !== "true";
}

export function withSupervisorEnv(
  env: NodeJS.ProcessEnv,
  homeDir: string,
): NodeJS.ProcessEnv & {
  KANDEV_SUPERVISOR_SOCKET: string;
  KANDEV_SUPERVISOR_MANIFEST: string;
  KANDEV_RESTART_ADAPTER: string;
} {
  prepareSupervisorDir(homeDir);
  return {
    ...env,
    KANDEV_SUPERVISOR_SOCKET: socketPath(homeDir),
    KANDEV_SUPERVISOR_MANIFEST: manifestPath(homeDir),
    KANDEV_RESTART_ADAPTER: "supervisor",
  };
}

function resolveExecutable(command: string): string {
  if (path.isAbsolute(command)) return command;
  const found = spawnSync(process.platform === "win32" ? "where" : "which", [command], {
    encoding: "utf8",
  });
  const first = found.stdout
    ?.split(/\r?\n/)
    .map((line) => line.trim())
    .find(Boolean);
  if (!first) {
    throw new Error(`Unable to resolve executable ${command}`);
  }
  return first;
}
