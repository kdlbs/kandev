import { type Page } from "@playwright/test";
import { test, expect } from "../fixtures/test-base";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "../helpers/api-client";
import { SessionPage } from "../pages/session-page";

/**
 * Seed a task + session with a mock_remote executor and navigate to the session page.
 * Sets the workspace default executor to mock_remote so the session picks it up.
 */
async function seedRemoteSession(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
): Promise<{ session: SessionPage; sessionId: string }> {
  // Create a mock_remote executor and set it as workspace default
  const executor = await apiClient.createExecutor("E2E Mock Remote", "mock_remote");
  await apiClient.updateWorkspace(seedData.workspaceId, {
    default_executor_id: executor.id,
  });

  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );

  if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

  await testPage.goto(`/t/${task.id}`);

  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

  // Reset workspace default executor so other tests aren't affected
  await apiClient.updateWorkspace(seedData.workspaceId, {
    default_executor_id: "",
  });

  return { session, sessionId: task.session_id };
}

/**
 * Seed a task + session with the default (local) executor.
 */
async function seedLocalSession(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
): Promise<{ session: SessionPage; sessionId: string }> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );

  if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

  await testPage.goto(`/t/${task.id}`);

  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

  return { session, sessionId: task.session_id };
}

test.describe("Port Forward Dialog", () => {
  test("button is hidden for local executor session", async ({ testPage, apiClient, seedData }) => {
    const { session } = await seedLocalSession(testPage, apiClient, seedData, "Local Port Test");
    await expect(session.portForwardButton).not.toBeVisible();
  });

  test("button is visible for mock remote executor session", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { session } = await seedRemoteSession(testPage, apiClient, seedData, "Remote Port Test");
    await expect(session.portForwardButton).toBeVisible();
  });

  test("dialog opens on button click", async ({ testPage, apiClient, seedData }) => {
    const { session } = await seedRemoteSession(testPage, apiClient, seedData, "Dialog Open Test");
    await session.portForwardButton.click();
    await expect(session.portForwardDialog).toBeVisible();
  });

  test("auto-refresh loads port list on open", async ({ testPage, apiClient, seedData }) => {
    const { session } = await seedRemoteSession(testPage, apiClient, seedData, "Refresh Test");
    await session.portForwardButton.click();
    await expect(session.portForwardDialog).toBeVisible();
    // Dialog auto-refreshes on open; wait for the placeholder to disappear,
    // meaning either ports were detected or the "no ports" message appeared.
    // The E2E host may have real listening ports, so we accept either outcome.
    const noPortsMessage = session.portForwardDialog.getByText("No listening ports detected");
    const detectedBadge = session.portForwardDialog.getByText("Detected").first();
    await expect(noPortsMessage.or(detectedBadge)).toBeVisible({ timeout: 10_000 });
  });

  test("add manual port shows row with Manual badge", async ({ testPage, apiClient, seedData }) => {
    const { session } = await seedRemoteSession(testPage, apiClient, seedData, "Manual Port Test");
    await session.portForwardButton.click();
    await expect(session.portForwardDialog).toBeVisible();

    await session.portForwardInput.fill("3000");
    await session.portForwardAddButton.click();

    const row = session.portForwardRow(3000);
    await expect(row).toBeVisible();
    await expect(row.getByText("Manual")).toBeVisible();
  });

  test("rejects invalid port number", async ({ testPage, apiClient, seedData }) => {
    const { session } = await seedRemoteSession(testPage, apiClient, seedData, "Invalid Port Test");
    await session.portForwardButton.click();
    await expect(session.portForwardDialog).toBeVisible();

    await session.portForwardInput.fill("99999");
    await session.portForwardAddButton.click();

    // Invalid port should not create a row
    await expect(session.portForwardRow(99999)).not.toBeVisible();
  });

  test("rejects duplicate port", async ({ testPage, apiClient, seedData }) => {
    const { session } = await seedRemoteSession(
      testPage,
      apiClient,
      seedData,
      "Duplicate Port Test",
    );
    await session.portForwardButton.click();
    await expect(session.portForwardDialog).toBeVisible();

    await session.portForwardInput.fill("8080");
    await session.portForwardAddButton.click();
    await expect(session.portForwardRow(8080)).toBeVisible();

    // Try adding the same port again — should still have only one row
    await session.portForwardInput.fill("8080");
    await session.portForwardAddButton.click();
    const rows = session.portForwardDialog.locator('[data-testid="port-forward-row-8080"]');
    await expect(rows).toHaveCount(1);
  });

  test("manual port row has correct proxy URL", async ({ testPage, apiClient, seedData }) => {
    const { session, sessionId } = await seedRemoteSession(
      testPage,
      apiClient,
      seedData,
      "Proxy URL Test",
    );
    await session.portForwardButton.click();
    await expect(session.portForwardDialog).toBeVisible();

    await session.portForwardInput.fill("5000");
    await session.portForwardAddButton.click();

    const row = session.portForwardRow(5000);
    await expect(row).toBeVisible();

    const link = row.locator("a[target='_blank']");
    const href = await link.getAttribute("href");
    expect(href).toContain(`/port-proxy/${sessionId}/5000/`);
  });

  test("Enter key submits port", async ({ testPage, apiClient, seedData }) => {
    const { session } = await seedRemoteSession(testPage, apiClient, seedData, "Enter Key Test");
    await session.portForwardButton.click();
    await expect(session.portForwardDialog).toBeVisible();

    await session.portForwardInput.fill("4000");
    await session.portForwardInput.press("Enter");

    await expect(session.portForwardRow(4000)).toBeVisible();
  });
});
