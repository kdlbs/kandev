import { spawn } from "node:child_process";
import path from "node:path";

import { HEALTH_TIMEOUT_MS_DEV } from "./constants";
import { resolveHealthTimeoutMs, waitForHealth } from "./health";
import { createProcessSupervisor } from "./process";
import {
  attachBackendExitHandler,
  buildBackendEnv,
  buildWebEnv,
  logPortConfig,
  pickPorts,
} from "./shared";
import { launchWebApp } from "./web";

export type DevOptions = {
  repoRoot: string;
  backendPort?: number;
  webPort?: number;
};

export async function runDev({ repoRoot, backendPort, webPort }: DevOptions): Promise<void> {
  const ports = await pickPorts(backendPort, webPort);

  const backendEnv = buildBackendEnv({ ports, extra: { KANDEV_MOCK_AGENT: "true" } });
  const webEnv = buildWebEnv({ ports, includeMcp: true, debug: true });

  logPortConfig("dev", "using local repo", ports, true);

  const supervisor = createProcessSupervisor();
  supervisor.attachSignalHandlers();

  const backendProc = spawn("make", ["-C", path.join("apps", "backend"), "dev"], {
    cwd: repoRoot,
    env: backendEnv,
    stdio: "inherit",
  });
  supervisor.children.push(backendProc);

  attachBackendExitHandler(backendProc, supervisor);

  const healthTimeoutMs = resolveHealthTimeoutMs(HEALTH_TIMEOUT_MS_DEV);
  await waitForHealth(ports.backendUrl, backendProc, healthTimeoutMs);
  console.log(`[kandev] backend ready at ${ports.backendUrl}`);

  const webUrl = `http://localhost:${ports.webPort}`;
  launchWebApp({
    command: "pnpm",
    args: ["-C", "apps", "--filter", "@kandev/web", "dev"],
    cwd: repoRoot,
    env: webEnv,
    url: webUrl,
    supervisor,
    label: "web",
  });
  console.log(`[kandev] web ready at ${webUrl}`);
}
