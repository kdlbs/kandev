import { expect, test } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { attachMainFrameNavigationCapture } from "../../helpers/document-navigation";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import {
  countSessionStatusRequests,
  expectOwnedSessionStatusRequests,
} from "../../helpers/session-status-ownership";
import { attachSessionStatusRequestCapture } from "../../helpers/ws-capture";
import { SessionPage } from "../../pages/session-page";

type CreatedTask = {
  id: string;
  title: string;
  session_id?: string;
  primary_session_id?: string;
};

test.describe("Mobile subtask activation", () => {
  test("opens the created child conversation", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    test.setTimeout(180_000);
    // Distinct prompt markers prove the child chat is not the stale parent
    // conversation. Both resolve to the simple-message mock scenario.
    const runMarker = Date.now();
    const parentTitle = `Mobile activation parent ${runMarker}`;
    const parentPrompt = `/e2e:simple-message parent-conversation-${runMarker}`;
    const childPrompt = `/e2e:simple-message child-activation-${runMarker}`;
    const parent = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      parentTitle,
      seedData.agentProfileId,
      {
        description: parentPrompt,
        workflow_id: seedData.workflowId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.worktreeExecutorProfileId,
      },
    );
    await expect
      .poll(async () => (await apiClient.getTaskEnvironment(parent.id))?.status ?? null, {
        timeout: 30_000,
      })
      .toBe("ready");

    const statusRequests = attachSessionStatusRequestCapture(testPage);
    await testPage.goto(`/t/${parent.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // Existing phone entry: task picker sheet -> row actions -> Create Subtask.
    await testPage.getByTestId("mobile-task-picker-trigger").tap();
    const taskSheet = testPage.getByRole("dialog", { name: "Tasks" });
    const parentRow = taskSheet.getByTestId("sidebar-task-item").filter({ hasText: parentTitle });
    await expect(parentRow).toBeVisible();
    await parentRow.getByRole("button", { name: "Task actions" }).tap();
    await testPage.getByRole("menuitem", { name: "Create Subtask", exact: true }).tap();

    const dialog = testPage.getByTestId("new-subtask-dialog");
    const promptInput = dialog.getByTestId("subtask-prompt-input");
    await expect(promptInput).toBeVisible();
    await promptInput.fill(childPrompt);

    const createdResponse = waitForHttp(testPage, "POST", /\/api\/v1\/tasks$/);
    // Arm before submission: the handoff must stay client-side. Any main-frame
    // document navigation here would mean a reload recovered the page.
    const documentNavigations = attachMainFrameNavigationCapture(testPage);
    await dialog.getByRole("button", { name: "Create Subtask", exact: true }).tap();
    const created = (await (await createdResponse).json()) as CreatedTask;
    const childSessionId = created.session_id ?? created.primary_session_id;
    if (!childSessionId) throw new Error("creation must return the child session");
    await expect(dialog).toBeHidden({ timeout: 30_000 });

    // These assertions are passive: they cannot reload the document or recover
    // a broken handoff. The exact data-session-id and child-only prompt prove
    // the visible chat is the created child conversation.
    const childUrl = new RegExp(`/t/${created.id}\\?sessionId=${childSessionId}$`);
    await expect(testPage).toHaveURL(childUrl, { timeout: 15_000 });
    await expect(testPage.getByTestId("mobile-task-picker-trigger")).toContainText(created.title);
    const childChat = testPage.locator('[data-testid="session-chat"]:visible');
    await expect(childChat).toHaveCount(1);
    await expect(childChat).toHaveAttribute("data-session-id", childSessionId);
    await expect(childChat.getByText(childPrompt)).toBeVisible();
    await expect(childChat.getByText(parentPrompt)).toHaveCount(0);
    await expect(
      childChat.locator('.tiptap.ProseMirror[contenteditable="true"]').first(),
    ).toBeEditable();
    await expect(testPage.getByTestId("session-mobile-bottom-nav")).toBeVisible();
    await assertNoDocumentHorizontalOverflow(testPage, "phone child conversation");
    await expect(testPage.getByTestId("session-recovery-error")).toHaveCount(0);
    await expect(testPage.getByText(/does not belong to task/i)).toHaveCount(0);
    await expect
      .poll(() => countSessionStatusRequests(statusRequests.frames, created.id, childSessionId))
      .toBeGreaterThan(0);
    await expectOwnedSessionStatusRequests(apiClient, statusRequests.frames, [
      { taskId: parent.id },
      { taskId: created.id, sessionIds: [childSessionId] },
    ]);
    expect(
      documentNavigations.requests,
      `the child conversation must appear before any document reload: ${JSON.stringify(
        documentNavigations.requests,
      )}`,
    ).toEqual([]);
    await prCapture.screenshot("phone-subtask-child-conversation", {
      caption: "Phone child task conversation opened directly after subtask creation",
    });

    // Reload after the handoff assertions as a separate persistence check and
    // repeat the exact child-session/content assertions.
    const childStatusRequestsBeforeReload = countSessionStatusRequests(
      statusRequests.frames,
      created.id,
      childSessionId,
    );
    await testPage.reload();
    const reloadedChildChat = testPage.locator('[data-testid="session-chat"]:visible');
    await expect(reloadedChildChat).toHaveCount(1);
    await expect(reloadedChildChat).toHaveAttribute("data-session-id", childSessionId);
    await expect(reloadedChildChat.getByText(childPrompt)).toBeVisible();
    await expect(reloadedChildChat.getByText(parentPrompt)).toHaveCount(0);
    await expect(testPage).toHaveURL(childUrl);
    await expect(testPage.getByTestId("mobile-task-picker-trigger")).toContainText(created.title);
    await expect
      .poll(() => countSessionStatusRequests(statusRequests.frames, created.id, childSessionId))
      .toBeGreaterThan(childStatusRequestsBeforeReload);
    await assertNoDocumentHorizontalOverflow(testPage, "reloaded phone child conversation");
    await expect(testPage.getByTestId("session-recovery-error")).toHaveCount(0);
    await expectOwnedSessionStatusRequests(apiClient, statusRequests.frames, [
      { taskId: parent.id },
      { taskId: created.id, sessionIds: [childSessionId] },
    ]);
  });
});
