import { expect, type Page } from "@playwright/test";
import type { ApiClient } from "./api-client";
import type { OfficeApiClient } from "./office-api-client";

export type AggregateNavigationTarget = {
  workspaceId: string;
  workspaceName: string;
  agentId: string;
  agentName: string;
};

export async function createAggregateNavigationTarget(
  apiClient: ApiClient,
  officeApi: OfficeApiClient,
  agentProfileId: string,
  workspaceName: string,
): Promise<AggregateNavigationTarget> {
  const agentName = "Aggregate target agent";
  const created = await officeApi.completeOnboarding({
    workspaceName,
    taskPrefix: "AGG",
    agentName,
    agentProfileId,
    executorPreference: "local_pc",
  });
  try {
    const { run_id: runId } = await apiClient.seedRun({
      agentProfileId,
      status: "finished",
    });
    await apiClient.seedActivity({
      workspaceId: created.workspaceId,
      actorType: "agent",
      actorId: created.agentId,
      action: "run_processed",
      targetType: "run",
      targetId: runId,
      runId,
    });
    return {
      workspaceId: created.workspaceId,
      workspaceName,
      agentId: created.agentId,
      agentName,
    };
  } catch (error) {
    await officeApi.deleteWorkspace(created.workspaceId, workspaceName).catch(() => {});
    throw error;
  }
}

export async function assertAggregateRunOpensInSourceWorkspace(
  page: Page,
  activeWorkspaceId: string,
  target: AggregateNavigationTarget,
  mobile = false,
): Promise<void> {
  await page.goto(`/office/overview?workspaceId=${encodeURIComponent(activeWorkspaceId)}`);
  const actor = page.getByText(target.agentName, { exact: true });
  await expect(actor).toBeVisible();
  const activityRow = actor.locator("xpath=../..");
  const runLink = activityRow.getByRole("link", { name: "Run" });
  await expect(runLink).toHaveAttribute("href", new RegExp(`workspaceId=${target.workspaceId}`));
  if (mobile) {
    const runLinkBox = await runLink.boundingBox();
    expect(runLinkBox?.height).toBeGreaterThanOrEqual(44);
    const documentWidth = await page.evaluate(() => ({
      scroll: document.documentElement.scrollWidth,
      client: document.documentElement.clientWidth,
    }));
    expect(documentWidth.scroll).toBeLessThanOrEqual(documentWidth.client);
  }

  await runLink.click();
  await expect(page).toHaveURL(new RegExp(`workspaceId=${target.workspaceId}`));
  await expect(page.getByTestId("agent-topbar-name")).toHaveText(target.agentName);
  await expect(page.getByText("Agent not found.")).toHaveCount(0);
}
