import type { Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";

export async function openNativeCodexProfile(page: Page, apiClient: ApiClient) {
  const { agents } = await apiClient.listAgents();
  const agent = agents.find((item) => item.name === "codex-app-server");
  if (!agent) throw new Error("Codex app-server profile was not available while its flag was on");
  const profile =
    agent.profiles[0] ??
    (await apiClient.createAgentProfile(agent.id, "Codex app-server E2E", {
      model: "gpt-5-codex",
    }));
  await page.goto(`/settings/agents/${agent.name}/profiles/${profile.id}`);
  return { agent, profile };
}

export async function stubNativeCodexModels(page: Page): Promise<void> {
  await page.route("**/api/v1/agent-models/codex-app-server**", async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname.endsWith("/resolve")) {
      const request = route.request().postDataJSON() as { model?: string };
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          agent_name: "codex-app-server",
          model: request.model ?? "gpt-5-codex",
          status: "ok",
          config_options: [],
          error: null,
        }),
      });
      return;
    }
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        agent_name: "codex-app-server",
        status: "ok",
        models: [{ id: "gpt-5-codex", name: "GPT-5 Codex", is_default: true, source: "dynamic" }],
        current_model_id: "gpt-5-codex",
        modes: [],
        commands: [],
        error: null,
      }),
    });
  });
}
