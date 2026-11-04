import { openMCPComposerCustomization } from "./mcp-composer";
import { expect, type Page } from "@playwright/test";
import type { ApiClient } from "./api-client";
import type { SeedData } from "../fixtures/test-base";
import { waitForSessionDone } from "./session";
import { SessionPage } from "../pages/session-page";

export async function checkMCPSelectionRefresh({
  page,
  apiClient,
  seedData,
  mobile,
}: {
  page: Page;
  apiClient: ApiClient;
  seedData: SeedData;
  mobile: boolean;
}) {
  await apiClient.createMCPServer(seedData.workspaceId, {
    runtime_name: `e2e-mcp-${mobile ? "mobile-task" : "task"}-refresh`,
    display_name: "Refresh selection MCP",
    execution_mode: "remote",
    transport: "streamable_http",
    configuration: { url: "https://example.test/refresh" },
    source: "custom",
  });
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "MCP refresh task",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("Missing refresh session");
  await waitForSessionDone(apiClient, task.id, task.session_id, "Waiting for refresh session");
  const path = `**/api/v1/task-sessions/${task.session_id}/mcp-selections*`;
  const held: (() => void)[] = [];
  let reads = 0;
  let writes = 0;
  await page.route(path, async (route) => {
    if (route.request().method() !== "GET") {
      writes++;
      return route.continue();
    }
    const response = await route.fetch();
    const body = await response.json();
    const index = ++reads;
    body.mcp_state = {
      desired_revision: 2,
      applied_revision: index >= 4 ? 2 : 1,
      apply_state: index >= 4 ? "applied" : "pending_idle",
    };
    if (index > 1) await new Promise<void>((release) => held.push(release));
    await route.fulfill({ response, json: body });
  });
  try {
    await page.goto(`/t/${task.id}`);
    const session = new SessionPage(page);
    await session.waitForLoad();
    const surface = await openMCPComposerCustomization(page);
    const marker = page.getByTestId("mcp-selection-marker");
    await expect(marker).toHaveAttribute("data-state", "pending");
    const search = surface.getByRole("textbox", { name: "Search MCP servers" });
    await search.fill("Refresh");
    const input = await search.elementHandle();
    const checkbox = await surface.getByRole("checkbox").elementHandle();
    for (let index = 0; index < 3; index++) {
      await expect.poll(() => held.length).toBe(1);
      await expect(search).toBeFocused();
      await expect(search).toHaveValue("Refresh");
      expect(await input!.evaluate((node) => node.isConnected)).toBe(true);
      expect(await checkbox!.evaluate((node) => node.isConnected)).toBe(true);
      await expect(surface.getByText("Loading MCP servers...", { exact: true })).toHaveCount(0);
      if (index === 2) {
        await surface.getByTestId("mcp-explorer-close").click();
        await expect(marker).toHaveAttribute("data-state", "pending");
      }
      held.shift()!();
    }
    await expect(marker).toHaveCount(0);
    await openMCPComposerCustomization(page);
    await expect(surface).toContainText("Setup up to date");
    await expect(search).toHaveValue("Refresh");
    expect(writes).toBe(0);
  } finally {
    for (const release of held.splice(0)) release();
    await page.unrouteAll({ behavior: "wait" });
  }
}
