import { test, expect } from "../../fixtures/test-base";
import { openTaskSession, waitForLatestSessionDone } from "../../helpers/session";

test.describe("paste strips rich text formatting", () => {
  test("keeps a copied hyperlink's URL instead of its visible title", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Paste hyperlink keeps URL",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await waitForLatestSessionDone(apiClient, task.id, 1, "Wait for paste-formatting task");
    const session = await openTaskSession(testPage, task.id);
    await session.waitForChatIdle({ timeout: 30_000 });

    const editor = session.activeChat().locator(".tiptap.ProseMirror").first();
    await expect(editor).toBeEditable();
    await editor.click();

    const url =
      "https://dev.azure.com/ptbcp/IT.DIT/_sprints/taskboard/ATMOffer/IT.DIT/DIT/2026/2Weeks/2W.2026.17?workitem=1103594";
    await editor.evaluate((element, pastedUrl) => {
      const clipboardData = new DataTransfer();
      clipboardData.setData("text/plain", pastedUrl);
      clipboardData.setData(
        "text/html",
        `<a href="${pastedUrl}">ATMOffer 2W.2026.17 Taskboard - Boards</a>`,
      );
      const pasteEvent = new Event("paste", { bubbles: true, cancelable: true });
      Object.defineProperty(pasteEvent, "clipboardData", { value: clipboardData });
      element.dispatchEvent(pasteEvent);
    }, url);

    await expect(editor).toContainText(url);
    await expect(editor).not.toContainText("Taskboard - Boards");
  });
});
