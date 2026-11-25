import { test, expect } from "../../fixtures/test-base";
import { visitOtherBrowserTab } from "../../helpers/external-focus";
import { openQuickChatSetup, startQuickChatFromSetup } from "../chat/quick-chat-helpers";

test.use({ channel: "chromium" });

// @covers AC-UI-EDIT-FOCUS-001.1, AC-UI-EDIT-FOCUS-001.2
test("Quick Chat rename retains an unfinished name across browser tabs", async ({
  testPage,
  apiClient,
}) => {
  const dialog = await openQuickChatSetup(testPage);
  const started = await startQuickChatFromSetup(dialog, testPage, "/e2e:simple-message");
  const tab = dialog
    .locator(`[data-tab-reference="conversation:${started.session_id}"]`)
    .getByTestId("quick-chat-tab");
  const originalTitle = (await apiClient.getTask(started.task_id)).title;
  await tab.locator('button[title="Double-click to rename"]').dblclick();
  const input = tab.getByRole("textbox", { name: "Rename chat" });
  await input.fill("Release notes");
  await input.evaluate((el) => (el as HTMLInputElement).setSelectionRange(7, 7));
  await visitOtherBrowserTab(testPage, async () => {
    await expect(input).toHaveValue("Release notes");
    expect((await apiClient.getTask(started.task_id)).title).toBe(originalTitle);
  });
  await testPage.keyboard.type(" draft");
  await expect(input).toHaveValue("Release draft notes");
  await input.press("Enter");
  await expect
    .poll(async () => (await apiClient.getTask(started.task_id)).title)
    .toBe("Release draft notes");
});
