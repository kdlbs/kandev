import { expect, type Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import { waitForLatestSessionDone } from "../../helpers/session";
import type { SeedData } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

export function startSettingsSession(apiClient: ApiClient, seedData: SeedData) {
  return apiClient.startConfigChat(
    seedData.workspaceId,
    seedData.agentProfileId,
    [
      'e2e:message("Settings flow started")',
      'e2e:mcp:kandev:search_settings_kandev({"query":"terminal font size","limit":10})',
      'e2e:mcp:kandev:describe_setting_kandev({"key":"user_settings.terminal_font_size","operation":"update"})',
      `e2e:mcp:kandev:list_settings_resources_kandev({"resource_type":"workflow","workspace_id":"${seedData.workspaceId}","limit":10})`,
      `e2e:mcp:kandev:get_settings_kandev({"target":{"resource_type":"agent_profile","resource_id":"${seedData.agentProfileId}"},"keys":["name","model"]})`,
      `e2e:mcp:kandev:update_settings_kandev({"target":{"resource_type":"agent_profile","resource_id":"${seedData.agentProfileId}"},"changes":{"name":"Agent settings E2E"}})`,
      'e2e:message("Settings flow complete")',
    ].join("\n"),
  );
}

export async function waitForSettingsSession(testPage: Page, apiClient: ApiClient, taskId: string) {
  await waitForLatestSessionDone(
    apiClient,
    taskId,
    1,
    "settings MCP session should complete before reading the result",
    30_000,
  );
  await testPage.goto(`/t/${taskId}`);
  const page = new SessionPage(testPage);
  await page.waitForLoad();
  await page.waitForChatIdle({ timeout: 30_000 });
  await expect(page.activeChat().getByText("Settings flow complete", { exact: true })).toBeVisible({
    timeout: 60_000,
  });
  return page;
}

export async function expandSettingsToolCalls(page: SessionPage) {
  const headers = page.chat.getByRole("button", { name: /\d+ tool calls?$/ });
  for (let index = 0; index < (await headers.count()); index++) {
    await headers.nth(index).click();
  }
}
