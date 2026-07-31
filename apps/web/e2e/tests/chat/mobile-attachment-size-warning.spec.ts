import { test, expect } from "../../fixtures/test-base";
import { openTaskSession, waitForLatestSessionDone } from "../../helpers/session";

const OVERSIZED_FILE_BYTES = 11 * 1024 * 1024;

test.describe("oversized attachment feedback on mobile", () => {
  test("warns when a pasted file exceeds the attachment limit", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Oversized pasted attachment warning",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await waitForLatestSessionDone(apiClient, task.id, 1, "Wait for attachment warning task");
    const session = await openTaskSession(testPage, task.id);
    await session.waitForChatIdle({ timeout: 30_000 });

    const editor = session.activeChat().locator(".tiptap.ProseMirror").first();
    await expect(editor).toBeEditable();
    await editor.evaluate((element, fileSize) => {
      const clipboardData = new DataTransfer();
      clipboardData.items.add(
        new File([new Uint8Array(fileSize)], "recording.mov", {
          type: "video/quicktime",
        }),
      );
      const pasteEvent = new Event("paste", { bubbles: true, cancelable: true });
      Object.defineProperty(pasteEvent, "clipboardData", { value: clipboardData });
      element.dispatchEvent(pasteEvent);
    }, OVERSIZED_FILE_BYTES);

    const warning = testPage
      .getByTestId("toast-message")
      .filter({ hasText: "Attachment is too large" });
    await expect(warning).toContainText(
      "recording.mov is 11.0 MB. The maximum file size is 10.0 MB.",
    );

    await expect
      .poll(async () => {
        const [warningBox, viewport] = await Promise.all([
          warning.boundingBox(),
          Promise.resolve(testPage.viewportSize()),
        ]);
        return (
          warningBox !== null &&
          viewport !== null &&
          warningBox.x >= 0 &&
          warningBox.x + warningBox.width <= viewport.width
        );
      })
      .toBe(true);
    await expect
      .poll(() =>
        testPage.evaluate(
          () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
        ),
      )
      .toBe(true);
  });
});
