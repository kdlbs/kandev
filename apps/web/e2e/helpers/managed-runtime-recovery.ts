import { expect } from "@playwright/test";
import { createHash } from "node:crypto";
import type { BackendContext } from "../fixtures/backend";
import type { ApiClient } from "./api-client";
import type { AgentProfile } from "../../lib/types/http-agents";

export const MANAGED_RUNTIME_CACHE_ROOT = "/tmp/kandev-managed-npm-cache";
const MANAGED_RUNTIME_AGENT_NAME = "opencode-acp";

export function managedRuntimeExecutionCacheKey(packageSpec: string): string {
  return createSha512(packageSpec).slice(0, 16);
}

export type ManagedRuntimePreparation = {
  profile: AgentProfile;
  packageSpec: string;
};

/**
 * The real managed OpenCode agent is enabled only for this container-backed
 * test. Its command runs through the image's npx wrapper, while the wrapper
 * starts the Linux mock ACP binary on the online retry.
 */
export async function prepareManagedRuntimeProfile(
  apiClient: ApiClient,
  backend: BackendContext,
): Promise<ManagedRuntimePreparation> {
  await backend.restart({ KANDEV_MOCK_AGENT: "true" });

  let agentId = "";
  let packageSpec = "";
  let observedAgents = "";
  try {
    await expect
      .poll(
        async () => {
          const { agents } = await apiClient.listAvailableAgents();
          observedAgents = agents
            .map((agent) => {
              const runtime = agent.runtime_update;
              const version = runtime?.effective_version ? `@${runtime.effective_version}` : "";
              return `${agent.name}:${agent.available ? "available" : "unavailable"}${version}`;
            })
            .join(", ");
          const managedAgent = agents.find(
            (agent) =>
              agent.name === MANAGED_RUNTIME_AGENT_NAME &&
              agent.available &&
              agent.runtime_update?.supported &&
              agent.runtime_update.package &&
              agent.runtime_update.effective_version,
          );
          agentId = managedAgent?.name ?? "";
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

  return {
    profile: await apiClient.createAgentProfile(agentId, "E2E managed npm recovery", {
      model: "mock-fast",
      env_vars: [{ key: "NPM_CONFIG_CACHE", value: MANAGED_RUNTIME_CACHE_ROOT }],
    }),
    packageSpec,
  };
}

/** Restore the normal e2e-only mock registry after a managed-runtime test. */
export async function restoreE2EAgentRegistry(backend: BackendContext): Promise<void> {
  await backend.restart();
}

function createSha512(value: string): string {
  return createHash("sha512").update(value).digest("hex");
}
