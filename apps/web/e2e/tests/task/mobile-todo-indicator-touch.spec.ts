import { test, expect } from "../../fixtures/test-base";
import { expectTodoDisclosure, openTodoTask } from "../../helpers/todo-disclosure";

// @covers AC-UI-AGENT-TODO-LIST-PANEL-002.2
test("phone tap opens and dismisses session todos", async ({
  testPage: page,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(90_000);
  const trigger = await openTodoTask(page, apiClient, seedData);
  expect(await page.evaluate(() => matchMedia("(pointer: coarse)").matches)).toBe(true);
  const box = await trigger.boundingBox();
  expect(box).not.toBeNull();
  expect(box!.width).toBeGreaterThanOrEqual(44);
  expect(box!.height).toBeGreaterThanOrEqual(44);
  await trigger.tap();
  const content = await expectTodoDisclosure(page, "touch");
  await prCapture.screenshot("phone-todos", { caption: "Phone showing the session checklist" });
  await trigger.tap();
  await expect(content).toBeHidden();
  await expect(trigger).toBeFocused();
});
