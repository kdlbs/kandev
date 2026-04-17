import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";
import type { Page } from "@playwright/test";

async function openTaskSession(page: Page, title: string): Promise<SessionPage> {
  const kanban = new KanbanPage(page);
  await kanban.goto();

  const card = kanban.taskCardByTitle(title);
  await expect(card).toBeVisible({ timeout: 15_000 });
  await card.click();
  await expect(page).toHaveURL(/\/t\//, { timeout: 15_000 });

  const session = new SessionPage(page);
  await session.waitForLoad();
  return session;
}

test.describe("Session resume boot-message dedup", () => {
  // Test restarts the backend multiple times — can be flaky under CI load.
  test.describe.configure({ retries: 1 });

  test("only the most recent 'Resumed agent' boot message is visible", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(180_000);

    // 1. Create the task and wait for the initial agent turn to finish.
    await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Resume Dedup Task",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    const session = await openTaskSession(testPage, "Resume Dedup Task");
    await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 30_000,
    });
    await expect(session.idleInput()).toBeVisible({ timeout: 15_000 });

    // 2. Initial "Started agent Mock" row should be visible exactly once.
    await expect(session.chat.getByText("Started agent Mock", { exact: false })).toHaveCount(1, {
      timeout: 15_000,
    });

    // 3. Restart the backend three times to produce three "Resumed agent" boot
    //    messages. The first two must be hidden by the dedup; only the third
    //    (most recent) should remain visible.
    for (let i = 0; i < 3; i++) {
      await backend.restart();
      await testPage.reload();
      await session.waitForLoad();
      // Auto-resume + agent turn completion can be slow under CI load.
      await expect(session.idleInput()).toBeVisible({ timeout: 60_000 });
      await expect(session.chat.getByText("Resumed agent Mock", { exact: false })).toBeVisible({
        timeout: 30_000,
      });
    }

    // 4. Key assertion: despite three resumes, only the last "Resumed agent"
    //    row should be rendered.
    await expect(session.chat.getByText("Resumed agent Mock", { exact: false })).toHaveCount(1, {
      timeout: 15_000,
    });

    // 5. The original "Started agent" row must still be present — dedup must
    //    not affect non-resuming boot messages.
    await expect(session.chat.getByText("Started agent Mock", { exact: false })).toHaveCount(1, {
      timeout: 15_000,
    });

    // 6. Agent interaction still works after dedup.
    await session.sendMessage("/e2e:simple-message");
    await expect(
      session.chat.getByText("simple mock response", { exact: false }).nth(1),
    ).toBeVisible({ timeout: 30_000 });
  });
});
