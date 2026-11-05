import { expect, type Page, type Route } from "@playwright/test";
import type { ApiClient } from "./api-client";
import type { SeedData } from "../fixtures/test-base";
import { waitForSessionDone } from "./session";
import { SessionPage } from "../pages/session-page";
import { openMCPComposerCustomization } from "./mcp-composer";

export async function checkMCPSessionClarity({
  page,
  apiClient,
  seedData,
}: {
  page: Page;
  apiClient: ApiClient;
  seedData: SeedData;
}) {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Deferred MCP session",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("Missing clarity test session");
  await waitForSessionDone(apiClient, task.id, task.session_id, "Waiting for clarity session");
  const url = `**/api/v1/task-sessions/${task.session_id}/mcp-selections*`;
  const activeRoutes = new Set<Promise<void>>();
  const handler = async (route: Route) => {
    let finish!: () => void;
    const finished = new Promise<void>((resolve) => {
      finish = resolve;
    });
    activeRoutes.add(finished);
    try {
      if (route.request().method() !== "GET") {
        await route.continue();
        return;
      }
      const response = await route.fetch();
      const body = await response.json();
      body.mcp_state = {
        desired_revision: 3,
        applied_revision: 1,
        apply_state: "deferred_restart",
      };
      await route.fulfill({ response, json: body });
    } finally {
      activeRoutes.delete(finished);
      finish();
    }
  };
  await page.route(url, handler);
  try {
    await page.goto(`/t/${task.id}`);
    await new SessionPage(page).waitForLoad();
    const surface = await openMCPComposerCustomization(page);
    await expect(surface).toContainText("Saved for the next agent start");
    await expect(surface).toContainText("Additional servers");
    await expect(surface).toContainText(
      "Your current setup stays in use. Saved changes apply when the agent starts again.",
    );
    await expect(surface).not.toContainText("Changes apply when the session is idle.");
    await expect(surface).not.toContainText("Desired revision");
    const marker = page.getByTestId("mcp-selection-marker");
    await expect(marker).toHaveAttribute("data-state", "deferred");
    await surface.getByTestId("mcp-explorer-close").click();
    await expect(marker).toHaveAttribute("data-state", "deferred");
    await openMCPComposerCustomization(page);
    await expect(surface).toContainText("Additional servers");
  } finally {
    // Removing an active route continues its request before our callback can fulfill it.
    while (activeRoutes.size > 0) await Promise.all([...activeRoutes]);
    await page.unroute(url, handler);
  }
}
