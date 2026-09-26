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

    const url = "https://example.com/boards/sprint/42?workitem=1234";
    const title = "Example Board Sprint 42";
    await editor.evaluate(
      (element, data) => {
        const clipboardData = new DataTransfer();
        clipboardData.setData("text/plain", data.title);
        clipboardData.setData("text/html", `<a href="${data.url}">${data.title}</a>`);
        const pasteEvent = new Event("paste", { bubbles: true, cancelable: true });
        Object.defineProperty(pasteEvent, "clipboardData", { value: clipboardData });
        element.dispatchEvent(pasteEvent);
      },
      { url, title },
    );

    await expect(editor).toContainText(url);
    await expect(editor).not.toContainText(title);
  });
});
