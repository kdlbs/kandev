import { expect } from "@playwright/test";
import { createHash, randomUUID } from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import type { BackendContext } from "../fixtures/backend";
import type { ApiClient } from "./api-client";
import type { AgentProfile } from "../../lib/types/http-agents";

export const MANAGED_RUNTIME_CACHE_ROOT = "/tmp/kandev-managed-npm-cache";
const MANAGED_RUNTIME_AGENT_NAME = "opencode-acp";
const MANAGED_RUNTIME_TEST_MODEL = "opencode/big-pickle";

export function managedRuntimeExecutionCacheKey(packageSpec: string): string {
  return createSha512(packageSpec).slice(0, 16);
}

export function managedRuntimeStartupAttemptFile(launchId: string): string {
  const safeLaunchId = launchId.replace(/[^A-Za-z0-9_-]/g, "_");
  return path.join(MANAGED_RUNTIME_CACHE_ROOT, "kandev-e2e-attempts", safeLaunchId);
}

export function managedRuntimeStartupSentinels(packageSpec: string, launchId: string) {
  const safeLaunchId = launchId.replace(/[^A-Za-z0-9_-]/g, "_");
  const cacheRoot = path.join(MANAGED_RUNTIME_CACHE_ROOT, "_npx");
  return {
    selected: path.join(
      cacheRoot,
      managedRuntimeExecutionCacheKey(packageSpec),
      `retry-sentinel-${safeLaunchId}`,
    ),
    sibling: path.join(cacheRoot, "0123456789abcdef", `sibling-sentinel-${safeLaunchId}`),
  };
}

export type ManagedRuntimePreparation = {
  profile: AgentProfile;
  packageSpec: string;
  hostFixturePath?: string;
};

export type ManagedRuntimeStartupFixtureMode =
  | "success"
  | "transient"
  | "silent-exit"
  | "permanent"
  | "repeat-failure";

export type ManagedRuntimePreparationOptions = {
  startupMode?: ManagedRuntimeStartupFixtureMode;
  launchId?: string;
  hostSubprocess?: boolean;
};

/**
 * The real managed OpenCode agent is enabled only for this container-backed
 * test. Its command runs through the image's npx wrapper, while the wrapper
 * starts the Linux mock ACP binary on the online retry.
 */
export async function prepareManagedRuntimeProfile(
  apiClient: ApiClient,
  backend: BackendContext,
  options: ManagedRuntimePreparationOptions = {},
): Promise<ManagedRuntimePreparation> {
  const mockAgentPath = path.resolve(__dirname, "../../../backend/bin/mock-agent");
  let hostFixturePath: string | undefined;
  if (options.hostSubprocess) {
    const fixtureBin = path.join(backend.tmpDir, "bin");
    hostFixturePath = path.join(fixtureBin, "npx");
    fs.mkdirSync(fixtureBin, { recursive: true });
    fs.copyFileSync(path.resolve(__dirname, "../fixtures/managed-runtime-npx.sh"), hostFixturePath);
    fs.chmodSync(hostFixturePath, 0o755);
  }
  const inheritedPath = (process.env.PATH ?? "").split(path.delimiter).filter(Boolean);
  const pathWithoutNativeOpenCode = inheritedPath.filter(
    (directory) => !fs.existsSync(path.join(directory, "opencode")),
  );
  await backend.restart({
    KANDEV_MOCK_AGENT: "true",
    NPM_CONFIG_CACHE: MANAGED_RUNTIME_CACHE_ROOT,
    PATH: [
      path.join(backend.tmpDir, "bin"),
      path.resolve(__dirname, "../../../backend/bin"),
      ...pathWithoutNativeOpenCode,
    ].join(path.delimiter),
  });

  let agentId = "";
  let packageSpec = "";
  let observedAgents = "";
  try {
    await expect
      .poll(
        async () => {
          const [{ agents: availableAgents }, { agents: persistedAgents }] = await Promise.all([
            apiClient.listAvailableAgents(),
            apiClient.listAgents(),
          ]);
          observedAgents = availableAgents
            .map((agent) => {
              const runtime = agent.runtime_update;
              const version = runtime?.effective_version ? `@${runtime.effective_version}` : "";
              const model = agent.model_config.available_models[0]?.id ?? "none";
              return `${agent.name}:${agent.available ? "available" : "unavailable"}${version}:model=${model}`;
            })
            .join(", ");
          const managedAgent = availableAgents.find(
            (agent) =>
              agent.name === MANAGED_RUNTIME_AGENT_NAME &&
              agent.available &&
              agent.runtime_update?.supported &&
              agent.runtime_update.package &&
              agent.runtime_update.effective_version,
          );
          const persistedAgent = persistedAgents.find(
            (agent) => agent.name === MANAGED_RUNTIME_AGENT_NAME,
          );
          agentId = persistedAgent?.id ?? "";
          packageSpec = managedAgent?.runtime_update
            ? `${managedAgent.runtime_update.package}@${managedAgent.runtime_update.effective_version}`
            : "";
          return agentId && packageSpec ? `${agentId}:${packageSpec}` : "";
        },
        {
          timeout: 30_000,
          message: "OpenCode managed runtime metadata should be available for container recovery",
        },
      )
      .not.toBe("");
  } catch (error) {
    throw new Error(
      `${error instanceof Error ? error.message : String(error)}; agents=${observedAgents}`,
    );
  }

  const envVars = [{ key: "NPM_CONFIG_CACHE", value: MANAGED_RUNTIME_CACHE_ROOT }];
  if (options.startupMode) {
    envVars.push({ key: "E2E_NPX_MODE", value: options.startupMode });
    envVars.push({ key: "E2E_NPX_LAUNCH_ID", value: options.launchId ?? randomUUID() });
  }
  if (options.hostSubprocess) {
    envVars.push({
      key: "E2E_MOCK_AGENT_PATH",
      value: mockAgentPath,
    });
  }

  return {
    profile: await apiClient.createAgentProfile(agentId, "E2E managed npm recovery", {
      model: MANAGED_RUNTIME_TEST_MODEL,
      env_vars: envVars,
    }),
    packageSpec,
    hostFixturePath,
  };
}

/** Restore the normal e2e-only mock registry after a managed-runtime test. */
export async function restoreE2EAgentRegistry(backend: BackendContext): Promise<void> {
  await backend.restart();
}

function createSha512(value: string): string {
  return createHash("sha512").update(value).digest("hex");
}
