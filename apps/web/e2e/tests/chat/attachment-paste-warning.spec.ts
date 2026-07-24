import { test, expect } from "../../fixtures/test-base";
import { openTaskSession, waitForLatestSessionDone } from "../../helpers/session";

test.describe("browser image paste feedback", () => {
  test("warns when copied image content has no readable file", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Unreadable pasted image warning",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await waitForLatestSessionDone(apiClient, task.id, 1, "Wait for paste warning task");
    const session = await openTaskSession(testPage, task.id);
    await session.waitForChatIdle({ timeout: 30_000 });

    const editor = session.activeChat().locator(".tiptap.ProseMirror").first();
    await expect(editor).toBeEditable();
    await editor.evaluate((element) => {
      const clipboardData = new DataTransfer();
      clipboardData.items.add(
        '<img src="https://images.example.test/large-image.png" alt="">',
        "text/html",
      );
      const pasteEvent = new Event("paste", { bubbles: true, cancelable: true });
      Object.defineProperty(pasteEvent, "clipboardData", { value: clipboardData });
      element.dispatchEvent(pasteEvent);
    });

    const warning = testPage
      .getByTestId("toast-message")
      .filter({ hasText: "Pasted image couldn’t be attached" });
    await expect(warning).toContainText("Save the image, then attach the file instead.");
  });
});
