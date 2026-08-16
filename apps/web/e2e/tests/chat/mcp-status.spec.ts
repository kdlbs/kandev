import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

const THIRD_PARTY_MCP_CONFIG = {
  enabled: true,
  servers: {
    filesystem: {
      type: "sse",
      url: "https://mcp.example.test/sse",
    },
  },
};

async function openChat(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
): Promise<SessionPage> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "MCP explorer test",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  return session;
}

async function loadKandevCatalog(session: SessionPage) {
  await session.sendMessage("e2e:mcp:kandev:list_workspaces_kandev({})");
  await session.waitForChatIdle({ timeout: 30_000 });
}

test("desktop MCP explorer shows Kandev tools and third-party limits", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}, testInfo) => {
  test.skip(testInfo.project.name !== "chromium", "desktop-only MCP explorer");
  await apiClient.updateAgentProfileMcpConfig(seedData.agentProfileId, THIRD_PARTY_MCP_CONFIG);

  try {
    const session = await openChat(testPage, apiClient, seedData);
    await loadKandevCatalog(session);

    const trigger = testPage.getByTestId("mcp-status-trigger");
    await expect(trigger).toBeVisible();
    await trigger.click();

    const explorer = testPage.getByTestId("mcp-server-explorer");
    await expect(explorer).toBeVisible();
    await expect(explorer.getByTestId("mcp-server-row-kandev")).toHaveAttribute(
      "aria-current",
      "true",
    );
    const toolRow = explorer.getByTestId("mcp-tool-row-create_task_kandev");
    await expect(toolRow).toBeVisible({ timeout: 30_000 });
    await expect(toolRow.locator("p")).toHaveText(/\S+/);
    await prCapture.screenshot("desktop-mcp-explorer-kandev", {
      caption: "Desktop MCP server explorer with the Kandev tool catalog",
    });

    await explorer.getByTestId("mcp-server-row-filesystem").click();
    await expect(
      explorer.getByText("Kandev does not inspect tools from this server."),
    ).toBeVisible();
    await expect(
      explorer.getByTestId("mcp-server-detail").getByText("Delivered, connection unverified"),
    ).toBeVisible();
  } finally {
    await apiClient.updateAgentProfileMcpConfig(seedData.agentProfileId, {
      enabled: false,
      servers: {},
    });
  }

  prCapture.flush();
});
