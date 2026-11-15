import { test, expect } from "../../fixtures/test-base";
import { TASK_TITLE, sendTaskMentions } from "./task-mention-helpers";

test.use({ trace: "retain-on-failure" });

// @covers AC-UI-CHAT-TASK-MENTION-001.1
// @covers AC-UI-CHAT-TASK-MENTION-001.2
// @covers AC-UI-CHAT-TASK-MENTION-001.3
test("sent task mentions keep their identity after refresh and rename", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  const { target, source, session, bubble } = await sendTaskMentions(testPage, apiClient, seedData);
  const chips = bubble.getByTestId("task-mention-chip");
  await expect(chips).toHaveCount(2);
  await expect(chips.first()).toHaveAttribute("href", `/t/${target.id}`);
  await expect(chips.first()).toHaveCSS("text-decoration-line", "none");
  await expect(bubble).toContainText("@ordinary");
  await expect(bubble.getByRole("link", { name: "@ordinary", exact: true })).toHaveCount(0);
  for (const theme of ["light", "dark"]) {
    await testPage.evaluate((value) => {
      document.documentElement.classList.toggle("dark", value === "dark");
    }, theme);
    await prCapture.screenshot(`desktop-task-mentions-${theme}`, {
      caption: `Task references in a sent chat bubble (${theme} theme)`,
    });
  }
  await apiClient.updateTaskTitle(target.id, "Resolved terminal retry investigation");
  await testPage.reload();
  await session.showSessionContext();
  const chip = testPage
    .getByTestId("user-message-bubble")
    .getByRole("link", { name: `@${TASK_TITLE}`, exact: true })
    .first();
  await expect(chip).toHaveAttribute("href", `/t/${target.id}`);
  await testPage.setViewportSize({ width: 700, height: 900 });
  await expect.poll(async () => (await chip.boundingBox())?.height).toBeGreaterThanOrEqual(44);
  await chip.focus();
  await expect(chip).toBeFocused();
  await chip.press("Enter");
  await expect(testPage).toHaveURL(new RegExp(`/t/${target.id}(?:\\?|$)`));
  await testPage.goBack();
  await expect(testPage).toHaveURL(new RegExp(`/t/${source.id}(?:\\?|$)`));
});
