import { openMCPComposerCustomization } from "./mcp-composer";
import { expect, type Page } from "@playwright/test";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import { waitForSessionDone } from "./session";
import { SessionPage } from "../pages/session-page";

export async function checkMCPSelectionRejection({
  page,
  apiClient,
  seedData,
  mobile,
  stale,
}: {
  page: Page;
  apiClient: ApiClient;
  seedData: SeedData;
  mobile: boolean;
  stale: boolean;
}) {
  const server = await apiClient.createMCPServer(seedData.workspaceId, {
    runtime_name: `e2e-mcp-${mobile ? "mobile-task" : "task"}-rejection`,
    display_name: "Rejected session MCP",
    description: "Selection rejection regression.",
    execution_mode: "remote",
    transport: "streamable_http",
    configuration: { url: "https://mcp.example.test/rejection" },
    source: "custom",
  });
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "MCP rejection task",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("Missing rejection task session");
  await waitForSessionDone(apiClient, task.id, task.session_id, "Waiting for rejection session");
  const before = await apiClient.getMCPSelections(
    "task_session",
    task.session_id,
    seedData.workspaceId,
  );
  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });

  const surface = await openMCPComposerCustomization(page);
  const url = `**/api/v1/task-sessions/${task.session_id}/mcp-selections`;
  await page.route(url, async (route) => {
    if (route.request().method() !== "PUT") return route.continue();
    await route.fulfill({
      status: 403,
      contentType: "application/json",
      body: JSON.stringify({
        error: stale ? "interim settings interlock required" : "forbidden",
        error_code: stale ? "interim_settings_interlock_required" : "forbidden",
      }),
    });
  });
  const response = page.waitForResponse(
    (item) =>
      item.url().includes(`/task-sessions/${task.session_id}/mcp-selections`) &&
      item.request().method() === "PUT",
  );
  await surface.getByText(server.display_name, { exact: true }).click();
  expect((await response).status()).toBe(403);
  const reload = page.getByTestId("backend-reload-required-alert");
  if (stale) {
    await expect(reload).toBeVisible();
    await expect(surface).toHaveCount(0);
    await expect(surface.getByRole("alert")).toHaveCount(0);
  } else {
    const row = surface.locator("label").filter({ hasText: server.display_name });
    await expect(row.getByRole("checkbox")).toBeChecked();
    await expect(row.getByRole("checkbox")).toBeDisabled();
    await expect(surface.getByRole("alert")).toHaveText(
      "This connection cannot save MCP selections. Your previous choices are unchanged.",
    );
    await expect(reload).toHaveCount(0);
  }
  expect(
    await apiClient.getMCPSelections("task_session", task.session_id, seedData.workspaceId),
  ).toEqual(before);
  await page.unroute(url);
}
