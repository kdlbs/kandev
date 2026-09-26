import { expect, test } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { SessionPage } from "../../pages/session-page";

type SessionModeStoreWindow = Window & {
  __KANDEV_E2E_STORE__?: {
    getState: () => {
      setSessionMode: (
        sessionId: string,
        modeId: string,
        modes: { id: string; name: string }[],
        requestedModeId?: string,
      ) => void;
    };
  };
};

test.describe("session mode mismatch on mobile", () => {
  test("shows both modes and keeps mode selection reachable by touch", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile session mode mismatch",
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
    await testPage.evaluate((sessionId) => {
      const store = (window as SessionModeStoreWindow).__KANDEV_E2E_STORE__;
      if (!store) throw new Error("E2E store bridge is unavailable");
      store.getState().setSessionMode(
        sessionId,
        "default",
        [
          { id: "default", name: "Default" },
          { id: "bypassPermissions", name: "Bypass Permissions" },
        ],
        "bypassPermissions",
      );
    }, task.session_id);

    const leftActions = testPage.getByTestId("mobile-chat-toolbar-left-actions");
    const trigger = leftActions.getByTestId("session-mode-selector");
    await expect(trigger).toContainText("Default");
    await trigger.tap();

    const picker = testPage.getByRole("dialog");
    await expect(picker).toBeVisible();
    await expect(picker).toContainText("Requested Bypass Permissions, running in Default.");
    await expect(picker).toContainText("Bypass Permissions");
    await expect(picker).toContainText("Default");

    const close = picker.getByRole("button", { name: "Close" });
    expect((await close.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    await close.tap();
    await expect(picker).not.toBeVisible();
    await expect(trigger).toBeFocused();

    let selectedModeId: string | undefined;
    await testPage.route(`**/api/v1/task-sessions/${task.session_id}/set-mode`, async (route) => {
      selectedModeId = (route.request().postDataJSON() as { mode_id?: string }).mode_id;
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ ok: true }),
      });
    });
    await trigger.tap();
    const scrollArea = picker.getByTestId("session-mode-picker-scroll");
    await expect(scrollArea).toBeVisible();
    const scrollStyle = await scrollArea.evaluate((element) => ({
      overflowY: getComputedStyle(element).overflowY,
      paddingBottom: Number.parseFloat(getComputedStyle(element).paddingBottom),
    }));
    expect(scrollStyle.overflowY).toBe("auto");
    expect(scrollStyle.paddingBottom).toBeGreaterThan(0);

    const alternateMode = picker.getByRole("button", { name: "Bypass Permissions" });
    expect((await alternateMode.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    await alternateMode.tap();
    await expect(picker).not.toBeVisible();
    await expect(trigger).toBeFocused();
    await expect.poll(() => selectedModeId).toBe("bypassPermissions");
    await assertNoDocumentHorizontalOverflow(testPage, "mobile session mode picker");
  });
});
