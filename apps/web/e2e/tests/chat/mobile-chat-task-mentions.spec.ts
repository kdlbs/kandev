import { test, expect } from "../../fixtures/test-base";
import { sendTaskMentions } from "./task-mention-helpers";

// @covers AC-UI-CHAT-TASK-MENTION-001.1
// @covers AC-UI-CHAT-TASK-MENTION-001.2
// @covers AC-UI-CHAT-TASK-MENTION-001.4
test("phone task mentions wrap with touch targets and open the exact task", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  const { target, source, bubble } = await sendTaskMentions(testPage, apiClient, seedData);
  const chips = bubble.getByTestId("task-mention-chip");
  await expect(chips).toHaveCount(2);
  const bounds = await bubble.boundingBox();
  expect(bounds).not.toBeNull();
  const boxes = await Promise.all((await chips.all()).map((chip) => chip.boundingBox()));
  for (const box of boxes) {
    expect(box).not.toBeNull();
    expect(box!.height).toBeGreaterThanOrEqual(44);
    expect(box!.x).toBeGreaterThanOrEqual(bounds!.x);
    expect(box!.x + box!.width).toBeLessThanOrEqual(bounds!.x + bounds!.width);
  }
  expect(boxes[1]!.y).toBeGreaterThanOrEqual(boxes[0]!.y + boxes[0]!.height);
  expect(await testPage.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(
    false,
  );
  await chips.first().scrollIntoViewIfNeeded();
  await prCapture.screenshot("phone-task-mentions", {
    caption: "Task chips wrap inside the phone chat bubble and open tasks with one tap",
  });
  await chips.first().tap();
  await expect(testPage).toHaveURL(new RegExp(`/t/${target.id}(?:\\?|$)`));
  await testPage.goBack();
  await expect(testPage).toHaveURL(new RegExp(`/t/${source.id}(?:\\?|$)`));
});
