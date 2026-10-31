import type { ApiClient } from "./api-client";
import type { Page } from "@playwright/test";
import type { CopilotPluginDiscoveryResponse } from "@/lib/types/copilot-plugin-discovery";

export const COPILOT_RECOVERY_PLUGIN_ID = "installed:copilot-plugins/spark";
export const COPILOT_RECOVERY_MODEL_ID = "fixture-copilot-model";
export const COPILOT_RECOVERY_CONFIG_OPTIONS = { reasoning_effort: "high" };

type CopilotPluginDiscoveryOptions = Partial<
  Pick<CopilotPluginDiscoveryResponse, "status" | "runtime_version" | "mount_permitted" | "plugins">
>;

export type CopilotPluginFixture = NonNullable<
  Awaited<ReturnType<typeof createCopilotPluginFixture>>
>;
type CopilotProbeScenario = "supported" | "capability" | "model-options";

type ProbeRequest = {
  profile_id?: string;
  refresh?: boolean;
  launch_settings?: {
    copilot_selected_plugins?: string[];
  };
};

type ResolveRequest = {
  profile_id?: string;
  model?: string;
  config_options?: Record<string, string>;
  launch_settings?: {
    copilot_selected_plugins?: string[];
  };
};

/**
 * Seed a concrete Copilot profile for plugin selection tests. The Copilot agent
 * row is built in; the spec skips when an E2E runtime does not register it.
 */
export async function createCopilotPluginFixture(apiClient: ApiClient) {
  const { agents } = await apiClient.listAgents();
  const agent = agents.find((candidate) => candidate.name === "copilot-acp");
  if (!agent) return null;
  const suffix = Date.now().toString(36);
  const profile = await apiClient.createAgentProfile(agent.id, `Copilot plugins ${suffix}`, {
    model: "auto",
    copilot_selected_plugins: [],
  });
  return {
    agentId: agent.id,
    agentName: agent.name,
    profileId: profile.id,
    profileName: profile.name,
    async dispose() {
      await apiClient.deleteAgentProfile(profile.id, true);
    },
  };
}

export async function requireCopilotPluginFixture(
  apiClient: ApiClient,
): Promise<CopilotPluginFixture> {
  const fixture = await createCopilotPluginFixture(apiClient);
  if (!fixture) {
    throw new Error("The E2E profile must register its disposable Copilot identity.");
  }
  return fixture;
}

export async function installCopilotPluginProbeRoutes(
  page: Page,
  fixture: CopilotPluginFixture,
  scenario: CopilotProbeScenario,
) {
  let recovered = false;
  const capabilityRequests: ProbeRequest[] = [];
  const modelOptionsRequests: ResolveRequest[] = [];
  const contextRevision = "copilot-policy-recovery-e2e";

  await page.route(`**/api/v1/agents/${fixture.agentId}/plugin-discovery`, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(
        copilotPluginDiscovery(fixture.agentId, fixture.profileId, {
          mount_permitted: scenario === "supported",
        }),
      ),
    });
  });
  await page.route(`**/api/v1/agent-models/${fixture.agentName}/probe`, async (route) => {
    capabilityRequests.push(route.request().postDataJSON() as ProbeRequest);
    const policyBlocked = scenario === "capability" && !recovered;
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        agent_name: fixture.agentName,
        status: policyBlocked ? "unsupported" : "ok",
        ...(policyBlocked ? { reason_code: "policy_unverified" } : {}),
        models: policyBlocked
          ? []
          : [
              {
                id: COPILOT_RECOVERY_MODEL_ID,
                name: COPILOT_RECOVERY_MODEL_ID,
                is_default: true,
                source: "dynamic",
              },
            ],
        current_model_id: policyBlocked ? undefined : COPILOT_RECOVERY_MODEL_ID,
        modes: [],
        commands: [],
        error: policyBlocked ? "Synthetic policy preflight diagnostic." : null,
        ...(policyBlocked ? {} : { context_revision: contextRevision }),
      }),
    });
  });
  await page.route(`**/api/v1/agent-models/${fixture.agentName}/resolve`, async (route) => {
    const request = route.request().postDataJSON() as ResolveRequest;
    modelOptionsRequests.push(request);
    const policyBlocked = scenario === "model-options" && !recovered;
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        agent_name: fixture.agentName,
        model: request.model ?? COPILOT_RECOVERY_MODEL_ID,
        status: policyBlocked ? "unsupported" : "ok",
        ...(policyBlocked ? { reason_code: "policy_unverified" } : {}),
        config_options: policyBlocked
          ? []
          : [
              {
                type: "select",
                id: "reasoning_effort",
                name: "Reasoning effort",
                current_value: COPILOT_RECOVERY_CONFIG_OPTIONS.reasoning_effort,
                options: [
                  { value: "low", name: "Low" },
                  { value: "high", name: "High" },
                ],
              },
            ],
        error: policyBlocked ? "Synthetic policy preflight diagnostic." : null,
        ...(policyBlocked ? {} : { context_revision: contextRevision }),
      }),
    });
  });

  return {
    capabilityRequests,
    modelOptionsRequests,
    allowRecovery() {
      recovered = true;
    },
  };
}

export function copilotPluginDiscovery(
  agentId: string,
  profileId: string,
  options: CopilotPluginDiscoveryOptions = {},
): CopilotPluginDiscoveryResponse {
  return {
    agent_id: agentId,
    profile_id: profileId,
    provider_id: "copilot-acp",
    status: options.status ?? "ready",
    runtime_version: options.runtime_version ?? "1.0.88",
    mount_permitted: options.mount_permitted ?? true,
    plugins: options.plugins ?? [
      {
        id: "installed:copilot-plugins/spark",
        name: "spark",
        marketplace: "copilot-plugins",
        version: "1.0.0",
        native_enabled: false,
        availability: "available",
      },
      {
        id: "live:fixture-market/harness",
        name: "harness",
        marketplace: "fixture-market",
        version: "1.2.0",
        native_enabled: true,
        availability: "available",
      },
      {
        id: "builtin:/browser-use",
        name: "browser-use",
        version: "0.0.4",
        native_enabled: false,
        availability: "unavailable",
        reason_code: "builtin_not_mountable",
      },
    ],
  };
}

export function copilotProfileSaveActions(page: Page) {
  const surface = page.getByTestId("settings-floating-save");
  return {
    surface,
    save: surface.getByRole("button", { name: /^Save( changes)?$/i }),
    reset: surface.getByRole("button", { name: /^Reset$/i }),
  };
}
