import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];

/**
 * Tests the session tabs on the kanban right-side preview panel:
 * - Every session of the task shows up as a tab
 * - Clicking a tab switches the rendered session body and updates the URL
 * - Closing a tab deletes the session and falls back to the next one
 */
test.describe("Preview session tabs", () => {
  test("shows all sessions as tabs and switches between them", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);

    // 1. Create a task — first session becomes primary.
    // Task descriptions use the scenario registry (`/e2e:<name>`), so we pick a
    // scenario with a unique, agent-only response string to avoid prompt/response
    // text collisions in `getByText` assertions.
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Preview Tabs Task",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    // 2. Wait for first session to finish.
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return DONE_STATES.includes(sessions[0]?.state);
        },
        { timeout: 30_000, message: "Waiting for first session to finish" },
      )
      .toBe(true);

    const { sessions: afterFirst } = await apiClient.listTaskSessions(task.id);
    const primaryId = afterFirst[0].id;

    // 3. Navigate to the full task view and launch a second session via the new-session dialog.
    // This mirrors the approach in preview-primary-session.spec.ts since there is no
    // dedicated API helper to start a second session on an existing task.
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const card = kanban.taskCardByTitle("Preview Tabs Task");
    await expect(card).toBeVisible({ timeout: 10_000 });
    await card.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 15_000,
    });

    await session.addPanelButton().click();
    await testPage.getByTestId("new-session-button").click();
    const dialog = testPage.getByRole("dialog");
    await expect(dialog).toBeVisible({ timeout: 5_000 });
    // Dialog prompts use the script command form; the agent echoes the argument.
    await dialog.locator("textarea").fill('e2e:message("secondary-session-response")');
    await dialog.getByRole("button").filter({ hasText: /Start/ }).click();
    await expect(dialog).not.toBeVisible({ timeout: 10_000 });

    // 4. Wait for the second session to finish (two sessions in a done state).
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return sessions.filter((s) => DONE_STATES.includes(s.state)).length;
        },
        { timeout: 60_000, message: "Waiting for second session to finish" },
      )
      .toBe(2);

    const { sessions: afterSecond } = await apiClient.listTaskSessions(task.id);
    const secondaryId = afterSecond.find((s) => s.id !== primaryId)?.id;
    if (!secondaryId) throw new Error("Secondary session not created");

    // The first session remains primary by default — creating a second via the
    // new-session dialog does not steal the primary flag (verified by
    // preview-primary-session.spec.ts).

    // 5. Enable preview-on-click and return to the kanban board.
    await apiClient.saveUserSettings({ enable_preview_on_click: true });
    await kanban.goto();

    const previewCard = kanban.taskCardByTitle("Preview Tabs Task");
    await expect(previewCard).toBeVisible({ timeout: 10_000 });
    await expect(previewCard.getByRole("button", { name: "Open full page" })).toBeVisible({
      timeout: 10_000,
    });
    await previewCard.click();

    // 6. Preview panel + both tabs are visible.
    const previewPanel = testPage.getByTestId("task-preview-panel");
    await expect(previewPanel).toBeVisible({ timeout: 10_000 });

    const primaryTab = testPage.getByTestId(`preview-session-tab-${primaryId}`);
    const secondaryTab = testPage.getByTestId(`preview-session-tab-${secondaryId}`);
    await expect(primaryTab).toBeVisible({ timeout: 10_000 });
    await expect(secondaryTab).toBeVisible();

    // 7. Primary tab is active by default and its session content is visible.
    // "simple mock response" appears only in the agent's reply, not in any prompt,
    // so the single getByText match is unambiguous.
    await expect(primaryTab).toHaveAttribute("data-state", "active");
    await expect(secondaryTab).toHaveAttribute("data-state", "inactive");
    await expect(previewPanel.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 15_000,
    });

    // 8. Click the secondary tab → content switches, URL updates.
    // The echoed marker "secondary-session-response" appears in both the user
    // prompt and the agent reply; `.first()` picks one deterministically and
    // is enough to prove the secondary session's body is rendered.
    await secondaryTab.click();
    await expect(secondaryTab).toHaveAttribute("data-state", "active");
    await expect(primaryTab).toHaveAttribute("data-state", "inactive");
    await expect(
      previewPanel.getByText("secondary-session-response", { exact: false }).first(),
    ).toBeVisible({ timeout: 15_000 });
    await expect(
      previewPanel.getByText("simple mock response", { exact: false }),
    ).not.toBeVisible();
    await expect(testPage).toHaveURL(new RegExp(`sessionId=${secondaryId}`), { timeout: 5_000 });

    // 9. Close the (non-active) primary tab via its x button.
    //    First switch back so the x is visible (alwaysShowClose only on the active tab),
    //    then hover the primary tab to reveal the hover x, then click it.
    //    Simpler: target the close testid directly — Playwright force-clicks through hover state.
    const primaryClose = testPage.getByTestId(`preview-session-tab-close-${primaryId}`);
    await primaryTab.hover();
    await primaryClose.click();

    // 10. Primary tab is gone; secondary remains and is active; content is secondary's.
    await expect(primaryTab).toHaveCount(0, { timeout: 10_000 });
    await expect(secondaryTab).toHaveAttribute("data-state", "active");
    await expect(
      previewPanel.getByText("secondary-session-response", { exact: false }).first(),
    ).toBeVisible();

    // 11. Backend reflects the deletion.
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return sessions.map((s) => s.id);
        },
        { timeout: 10_000, message: "Waiting for primary session to be deleted" },
      )
      .toEqual([secondaryId]);
  });
});
