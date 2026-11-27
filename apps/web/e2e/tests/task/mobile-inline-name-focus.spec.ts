import { test, expect } from "../../fixtures/test-base";
import { visitOtherBrowserTab } from "../../helpers/external-focus";
import { startQuickChatFromSetup } from "../chat/quick-chat-helpers";

test.use({ channel: "chromium" });

// @covers AC-UI-EDIT-FOCUS-001.6
test("phone Quick Chat rename survives an external focus change", async ({
  testPage,
  apiClient,
}) => {
  await testPage.goto("/");
  await testPage.getByTestId("app-nav-trigger").tap();
  await testPage.getByTestId("mobile-quick-chat-button").tap();
  const dialog = testPage.getByRole("dialog", { name: "Quick Chat" });
  const started = await startQuickChatFromSetup(dialog, testPage, "/e2e:simple-message");
  const tab = dialog
    .locator(`[data-tab-reference="conversation:${started.session_id}"]`)
    .getByTestId("quick-chat-tab");
  const originalTitle = (await apiClient.getTask(started.task_id)).title;
  await tab.getByRole("button", { name: /^Actions for / }).tap();
  await testPage.getByRole("menuitem", { name: "Rename", exact: true }).tap();
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
