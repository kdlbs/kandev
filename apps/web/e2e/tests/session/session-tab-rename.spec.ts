import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

test.describe("Session tab rename", () => {
  // @covers AC-UI-TASK-AGENT-TAB-RECONCILIATION-002.1 through .3
  test("keeps rename-input double-clicks out of the maximize handler", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Agent Tab Rename Double-click",
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
    await session.waitForChatIdle({ timeout: 30_000 });

    const tab = session.sessionTabBySessionId(task.session_id);
    await expect(tab).toBeVisible();
    const header = testPage.locator(
      `.dv-tabs-and-actions-container:has([data-testid="session-tab-${task.session_id}"])`,
    );
    const maximizeButton = header.getByTestId("dockview-maximize-btn");
    await expect(maximizeButton.locator(".tabler-icon-arrows-maximize")).toBeVisible();

    await tab.click({ button: "right" });
    await session.contextMenuItem("Rename").click();
    const input = testPage.getByTestId("session-tab-rename-input");
    await input.click();
    await expect(input).toBeFocused();
    await input.fill("implementation");
    await input.dblclick();
    await expect(maximizeButton.locator(".tabler-icon-arrows-maximize")).toBeVisible();
    const selection = await input.evaluate((element: HTMLInputElement) => ({
      start: element.selectionStart,
      end: element.selectionEnd,
      length: element.value.length,
    }));
    expect(selection).toEqual({ start: 0, end: 14, length: 14 });

    await testPage.keyboard.press("Escape");
    await expect(input).not.toBeVisible();
    await tab.dblclick();
    await expect(maximizeButton.locator(".tabler-icon-arrows-minimize")).toBeVisible();

    await tab.click({ button: "right" });
    await session.contextMenuItem("Rename").click();
    await input.click();
    await expect(input).toBeFocused();
    await input.fill("implementation");
    await input.dblclick();
    await expect(maximizeButton.locator(".tabler-icon-arrows-minimize")).toBeVisible();
    await testPage.keyboard.press("Escape");
  });
});
