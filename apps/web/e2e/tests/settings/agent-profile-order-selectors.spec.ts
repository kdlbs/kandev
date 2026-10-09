import type { Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import { SessionPage } from "../../pages/session-page";
import { openQuickChatSetup } from "../chat/quick-chat-helpers";
import {
  seedProfileOrder,
  selectorProfileNames,
  restoreProfileOrder,
} from "./agent-profile-order-helpers";

useRegularMode();

async function captureSelectors(page: Page, taskId: string, sessionId: string, names: string[]) {
  await page.goto("/");
  await page.getByTestId("create-task-button").first().click();
  const create = page.getByTestId("create-task-dialog");
  await expect(create).toBeVisible();
  const createSelector = create.getByTestId("agent-profile-selector");
  const taskOptions = await selectorProfileNames(page, createSelector, names);
  const taskDefault = await createSelector.textContent();
  await create.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(create).not.toBeVisible();

  const quickChat = await openQuickChatSetup(page);
  const quickSelector = quickChat.getByTestId("agent-profile-selector");
  const quickOptions = await selectorProfileNames(page, quickSelector, names);
  const quickDefault = await quickSelector.textContent();
  await page.keyboard.press("Escape");
  await expect(quickChat).not.toBeVisible();

  await page.goto(`/t/${taskId}?sessionId=${sessionId}`);
  const session = new SessionPage(page);
  await expect(session.sessionTabBySessionId(sessionId)).toBeVisible({ timeout: 30_000 });
  await session.openNewSessionDialog();
  const inTask = session.newSessionDialog();
  await expect(inTask).toBeVisible();
  const inTaskSelector = inTask.getByTestId("agent-profile-selector");
  const inTaskOptions = await selectorProfileNames(page, inTaskSelector, names);
  const inTaskDefault = await inTaskSelector.textContent();
  await inTask.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(inTask).not.toBeVisible();

  await session.sessionTabBySessionId(sessionId).click({ button: "right" });
  await session.handoffSubmenu().hover();
  await expect(page.locator('[data-testid^="handoff-profile-"]').first()).toBeVisible();
  const handoffLabels = await page.locator('[data-testid^="handoff-profile-"]').allTextContents();
  const handoffOptions = handoffLabels.flatMap((label) =>
    names.filter((name) => label.includes(name)),
  );
  await page.keyboard.press("Escape");
  return {
    taskDefault,
    taskOptions,
    quickDefault,
    quickOptions,
    inTaskDefault,
    inTaskOptions,
    handoffOptions,
  };
}

test("saved Settings order leaves task creation, quick chat, in-task and handoff choices unchanged", async ({
  testPage,
  apiClient,
  backend,
  seedData,
}) => {
  test.setTimeout(150_000);
  const { original, current, createdIds } = await seedProfileOrder(apiClient, "Selector");
  const names = current.profiles
    .filter((profile) => createdIds.includes(profile.id))
    .map((profile) => profile.name);
  const response = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Selector ordering isolation",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    },
  );
  const taskId = response.id;
  const sessionId = response.session_id!;
  try {
    await expect
      .poll(
        async () =>
          (await apiClient.listTaskSessions(taskId)).sessions.find(
            (session) => session.id === sessionId,
          )?.state,
        { timeout: 45_000 },
      )
      .toMatch(/COMPLETED|WAITING_FOR_INPUT/);
    const recentBefore = await apiClient.listAgentProfileRecentUse();
    const before = await captureSelectors(testPage, taskId, sessionId, names);
    expect(before.taskOptions).toEqual(names);
    expect(before.quickOptions).toEqual(names);
    expect(before.inTaskOptions).toEqual(names);
    expect(before.handoffOptions).toEqual(names);
    await restoreProfileOrder(testPage, backend.baseUrl, {
      ...current,
      profiles: [...current.profiles].reverse(),
    });
    const after = await captureSelectors(testPage, taskId, sessionId, names);
    expect(after).toEqual(before);
    expect(await apiClient.listAgentProfileRecentUse()).toEqual(recentBefore);
  } finally {
    await apiClient.deleteTask(taskId, { cascade: true });
    for (const id of createdIds) await apiClient.deleteAgentProfile(id, true);
    await restoreProfileOrder(testPage, backend.baseUrl, original);
  }
});
