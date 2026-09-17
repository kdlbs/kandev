import { test, expect } from "../../fixtures/test-base";
import { startQuickChatFromSetup } from "./quick-chat-helpers";
import {
  assertPickerContained,
  seedLongModelList,
  swipeModelList,
} from "./quick-chat-model-scroll-helpers";

test("touch scrolls Quick Chat models and selects a revealed model", async ({ testPage }) => {
  // @covers AC-UI-QUICK-CHAT-VIEWPORT-LAYOUT-001.7 AC-UI-QUICK-CHAT-VIEWPORT-LAYOUT-001.8
  await testPage.goto("/");
  await testPage.getByTestId("mobile-topbar-menu").tap();
  await testPage.getByTestId("mobile-quick-chat-button").tap();
  const dialog = testPage.getByRole("dialog", { name: "Quick Chat" });
  await startQuickChatFromSetup(dialog, testPage);
  const trigger = dialog.getByRole("button", { name: "Session model settings" });
  await expect(trigger).toContainText("Mock Fast");
  await trigger.tap();
  await seedLongModelList(testPage);
  const list = testPage.getByRole("listbox");
  const backgroundBefore = await testPage.evaluate(() => window.scrollY);
  const composerBefore = await dialog.locator(".tiptap.ProseMirror").boundingBox();
  await swipeModelList(testPage, list);
  await expect.poll(() => list.evaluate((element) => element.scrollTop)).toBeGreaterThan(0);
  const smart = list.getByRole("option", { name: /Mock Smart/ });
  await expect(async () => {
    await swipeModelList(testPage, list);
    await expect(smart).toBeInViewport();
  }).toPass({ timeout: 15_000 });
  await assertPickerContained(testPage);
  await smart.tap();
  await expect(trigger).toContainText("Mock Smart");
  expect(await testPage.evaluate(() => window.scrollY)).toBe(backgroundBefore);
  expect((await dialog.locator(".tiptap.ProseMirror").boundingBox())!.y).toBe(composerBefore!.y);
});
