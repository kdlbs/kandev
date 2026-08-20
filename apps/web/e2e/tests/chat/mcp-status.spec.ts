import { type Locator, type Page } from "@playwright/test";
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

async function expectRichStatusTooltip(testPage: Page, trigger: Locator) {
  await trigger.hover();
  const tooltip = testPage.getByRole("tooltip").getByTestId("mcp-status-popover");
  await expect(tooltip).toBeVisible();
  const kandev = tooltip.getByTestId("mcp-tooltip-server-kandev");
  await expect(kandev).toContainText("kandev");
  await expect(kandev).toContainText("Active");
  await expect(kandev.locator("span").first()).toHaveClass(/bg-emerald-500/);
}

async function expectExplorerTypography(elements: Locator[]) {
  const fontSizes = await Promise.all(
    elements.map((element) => element.evaluate((node) => getComputedStyle(node).fontSize)),
  );
  expect(new Set(fontSizes)).toEqual(new Set(["13px"]));
}

async function expectScrollAndFocusReturn(explorer: Locator) {
  const scroll = explorer.getByTestId("mcp-tool-list-scroll");
  const header = explorer.getByTestId("mcp-server-detail");
  const headerBefore = await header.boundingBox();
  const maxScroll = await scroll.evaluate((element) => element.scrollHeight - element.clientHeight);
  expect(maxScroll).toBeGreaterThan(0);
  await scroll.evaluate((element) => {
    element.scrollTop = element.scrollHeight;
  });
  await expect.poll(() => scroll.evaluate((element) => element.scrollTop)).toBeGreaterThan(0);
  const headerAfter = await header.boundingBox();
  expect(Math.abs((headerAfter?.y ?? 0) - (headerBefore?.y ?? 0))).toBeLessThanOrEqual(1);

  const lastTool = explorer.locator('[data-testid^="mcp-tool-row-"]').last();
  await lastTool.click();
  await expect(explorer.getByTestId("mcp-tool-detail")).toBeVisible();
  await explorer.getByRole("button", { name: "Back to tools" }).click();
  await expect(lastTool).toBeFocused();
  await expect.poll(() => scroll.evaluate((element) => element.scrollTop)).toBeGreaterThan(0);
}

test("desktop MCP explorer drills into tools and preserves list context", async ({
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
    await expectRichStatusTooltip(testPage, trigger);
    await trigger.click();

    const explorer = testPage.getByTestId("mcp-server-explorer");
    await expect(explorer).toBeVisible();
    await expect(explorer.getByRole("button", { name: "Close", exact: true })).toHaveCount(1);
    await expect(explorer.getByTestId("mcp-server-row-kandev")).toHaveAttribute(
      "aria-current",
      "true",
    );
    const createTask = explorer.getByTestId("mcp-tool-row-create_task_kandev");
    await expect(createTask).toBeVisible({ timeout: 30_000 });
    await expect(createTask.getByText(/^~\d+ tokens?$/)).toBeVisible();
    await expect(createTask.locator("p")).toHaveCount(0);
    await expectExplorerTypography([
      explorer.locator('[data-slot="dialog-description"]'),
      explorer.getByTestId("mcp-server-detail").locator("h3"),
      createTask,
      createTask.getByText(/^~\d+ tokens?$/),
    ]);
    await prCapture.screenshot("desktop-mcp-explorer-tools", {
      caption: "Desktop MCP explorer with compact tool rows and token estimates",
    });

    await createTask.click();
    const detail = explorer.getByTestId("mcp-tool-detail");
    await expect(detail).toBeVisible();
    await expect(detail.getByText(/^~\d+ tokens?$/)).toBeVisible();
    await expect(detail.locator("section").first().locator("p")).toHaveText(/\S+/);
    await expect(detail.getByText("title", { exact: true })).toBeVisible();
    await expectExplorerTypography([
      explorer.locator('[data-slot="dialog-description"]'),
      detail.locator("h3"),
      detail.locator("section").first().locator("p"),
      detail.getByText("title", { exact: true }),
    ]);
    await prCapture.screenshot("desktop-mcp-explorer-tool-detail", {
      caption: "Desktop MCP tool detail with description and arguments",
    });
    await detail.getByRole("button", { name: "Back to tools" }).click();
    await expect(createTask).toBeFocused();
    await expectScrollAndFocusReturn(explorer);

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
