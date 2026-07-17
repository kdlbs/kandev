import type { ApiClient } from "./api-client";
import type { OfficeApiClient } from "./office-api-client";

const PROFILE_WAIT_MS = 20_000;

export async function balancedExecutionProfileRouting(
  apiClient: ApiClient,
  officeApi: OfficeApiClient,
  workspaceId: string,
  providerOrder: string[],
) {
  const deadline = Date.now() + PROFILE_WAIT_MS;
  while (Date.now() < deadline) {
    const agents = await apiClient.listAgents();
    const profiles = await Promise.all(
      providerOrder.map(async (providerId) => {
        const agent = agents.agents.find((item) => item.name === providerId);
        if (!agent) return undefined;
        const existing = agent.profiles.find((profile) => profile.model !== "");
        if (existing) return { id: existing.id, model: existing.model };
        const model = `e2e-${providerId}-model`;
        const created = await apiClient.createAgentProfile(agent.id, `E2E ${providerId}`, {
          model,
        });
        return { id: created.id, model };
      }),
    );
    if (profiles.some((profile) => profile === undefined)) {
      await new Promise((resolve) => setTimeout(resolve, 250));
      continue;
    }
    const routing = await officeApi.getRouting(workspaceId);
    const selected = profiles.map((profile, index) =>
      routing.execution_profiles.find(
        (candidate) =>
          candidate.provider_id === providerOrder[index] && candidate.id === profile!.id,
      ),
    );
    if (selected.every((profile) => profile !== undefined)) {
      return {
        enabled: true,
        provider_order: providerOrder,
        default_tier: "balanced" as const,
        provider_profiles: Object.fromEntries(
          selected.map((profile, index) => [
            providerOrder[index],
            {
              tier_map: { balanced: profile!.model },
              execution_profile_ids: { balanced: profile!.id },
            },
          ]),
        ),
      };
    }
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
  throw new Error(`execution profiles did not become available for ${providerOrder.join(", ")}`);
}
