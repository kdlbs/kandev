import { spawn } from "node:child_process";
import path from "node:path";

import {
  DEFAULT_AGENTCTL_PORT,
  DEFAULT_BACKEND_PORT,
  DEFAULT_MCP_PORT,
  DEFAULT_WEB_PORT,
  HEALTH_TIMEOUT_MS,
} from "./constants";
import { waitForHealth } from "./health";
import { pickAvailablePort } from "./ports";
import { createProcessSupervisor } from "./process";
import { launchWebApp } from "./web";

export type DevOptions = {
  repoRoot: string;
  backendPort?: number;
  webPort?: number;
};

export async function runDev({ repoRoot, backendPort, webPort }: DevOptions): Promise<void> {
  const actualBackendPort = backendPort ?? (await pickAvailablePort(DEFAULT_BACKEND_PORT));
  const actualWebPort = webPort ?? (await pickAvailablePort(DEFAULT_WEB_PORT));
  const agentctlPort = await pickAvailablePort(DEFAULT_AGENTCTL_PORT);
  const mcpPort = await pickAvailablePort(DEFAULT_MCP_PORT);
  const backendUrl = `http://localhost:${actualBackendPort}`;
  const mcpUrl = `http://localhost:${mcpPort}/sse`;

  const backendEnv = {
    ...process.env,
    KANDEV_SERVER_PORT: String(actualBackendPort),
    KANDEV_AGENT_STANDALONE_PORT: String(agentctlPort),
    KANDEV_AGENT_MCP_SERVER_PORT: String(mcpPort),
  };
  const webEnv = {
    ...process.env,
    KANDEV_API_BASE_URL: backendUrl,
    NEXT_PUBLIC_KANDEV_API_BASE_URL: backendUrl,
    KANDEV_MCP_SERVER_URL: mcpUrl,
    NEXT_PUBLIC_KANDEV_MCP_SERVER_URL: mcpUrl,
    PORT: String(actualWebPort),
    NEXT_PUBLIC_KANDEV_DEBUG: "true",
  };

  console.log("[kandev] dev mode: using local repo");
  console.log("[kandev] backend port:", actualBackendPort);
  console.log("[kandev] web port:", actualWebPort);
  console.log("[kandev] agentctl port:", agentctlPort);
  console.log("[kandev] mcp port:", mcpPort);
  console.log("[kandev] mcp url:", mcpUrl);

  const supervisor = createProcessSupervisor();
  supervisor.attachSignalHandlers();

  const backendProc = spawn(
    "make",
    ["-C", path.join("apps", "backend"), "dev"],
    { cwd: repoRoot, env: backendEnv, stdio: "inherit" },
  );
  supervisor.children.push(backendProc);

  backendProc.on("exit", (code, signal) => {
    console.error(`[kandev] backend exited (code=${code}, signal=${signal})`);
    const exitCode = signal ? 0 : code ?? 1;
    void supervisor.shutdown("backend exit").then(() => process.exit(exitCode));
  });

  await waitForHealth(backendUrl, backendProc, HEALTH_TIMEOUT_MS);
  console.log(`[kandev] backend ready at ${backendUrl}`);

  const webUrl = `http://localhost:${actualWebPort}`;
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
