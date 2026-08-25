import { expect } from "@playwright/test";
import { createHash } from "node:crypto";
import type { BackendContext } from "../fixtures/backend";
import type { ApiClient } from "./api-client";
import type { AgentProfile } from "../../lib/types/http-agents";

export const MANAGED_RUNTIME_PACKAGE_SPEC = "opencode-ai@1.18.18";
export const MANAGED_RUNTIME_CACHE_ROOT = "/tmp/kandev-managed-npm-cache";

export function managedRuntimeExecutionCacheKey(
  packageSpec = MANAGED_RUNTIME_PACKAGE_SPEC,
): string {
  return createSha512(packageSpec).slice(0, 16);
}

/**
 * The real managed OpenCode agent is enabled only for this container-backed
 * test. Its command runs through the image's npx wrapper, while the wrapper
 * starts the Linux mock ACP binary on the online retry.
 */
export async function prepareManagedRuntimeProfile(
  apiClient: ApiClient,
  backend: BackendContext,
): Promise<AgentProfile> {
  await backend.restart({ KANDEV_MOCK_AGENT: "true" });

  let agentId = "";
  let observedAgents = "";
  try {
    await expect
      .poll(
        async () => {
          const { agents } = await apiClient.listAgents();
          observedAgents = agents.map((agent) => `${agent.id}:${agent.name}`).join(", ");
          agentId = agents.find((agent) => agent.name === "opencode-acp")?.id ?? "";
          return agentId;
        },
        {
          timeout: 30_000,
          message: "OpenCode managed runtime should be registered for container recovery",
        },
      )
      .not.toBe("");
  } catch (error) {
    throw new Error(
      `${error instanceof Error ? error.message : String(error)}; agents=${observedAgents}`,
    );
  }

  return apiClient.createAgentProfile(agentId, "E2E managed npm recovery", {
    model: "mock-fast",
    env_vars: [{ key: "NPM_CONFIG_CACHE", value: MANAGED_RUNTIME_CACHE_ROOT }],
  });
}

/** Restore the normal e2e-only mock registry after a managed-runtime test. */
export async function restoreE2EAgentRegistry(backend: BackendContext): Promise<void> {
  await backend.restart();
}

function createSha512(value: string): string {
  return createHash("sha512").update(value).digest("hex");
}
