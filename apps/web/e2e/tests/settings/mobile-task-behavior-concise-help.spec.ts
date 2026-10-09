import { expect, test } from "../../fixtures/test-base";

for (const width of [390, 767, 768, 900]) {
  test(`touch help preserves the draft and returns focus at ${width}px`, async ({ testPage }) => {
    await testPage.setViewportSize({ width, height: 844 });
    await testPage.goto("/settings/preferences/task-behavior");
    const toggle = testPage.locator("#creation-auto-focus");
    await toggle.tap();
    const draft = await toggle.getAttribute("aria-checked");
    const info = testPage.getByRole("button", {
      name: "About Profile for Tasks Created by Agents",
    });
    const box = await info.boundingBox();
    expect(box!.width).toBeGreaterThanOrEqual(44);
    expect(box!.height).toBeGreaterThanOrEqual(44);
    await info.tap();
    const drawer = testPage.getByRole("dialog", { name: "Profile for Tasks Created by Agents" });
    await expect(drawer).toBeVisible();
    await expect(drawer).toContainText("create_task_kandev");
    const drawerBox = await drawer.boundingBox();
    expect(drawerBox!.x).toBeGreaterThanOrEqual(0);
    expect(drawerBox!.x + drawerBox!.width).toBeLessThanOrEqual(width);
    await drawer.getByRole("button", { name: "Close", exact: true }).tap();
    await expect(info).toBeFocused();
    await expect(toggle).toHaveAttribute("aria-checked", draft!);
    await testPage.getByRole("tab", { name: "Runtime", exact: true }).tap();
    await expect(testPage.getByTestId("message-queue-max-per-session")).toBeVisible();
    await expect(testPage.getByTestId("task-behavior-settings").locator("details")).toHaveCount(0);
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);
    await testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Reset", exact: true })
      .tap();
  });
}

test("long translated descriptions and help stay inside the phone viewport", async ({
  testPage,
}) => {
  await testPage.goto("/settings/preferences/task-behavior");
  await testPage.evaluate(() => {
    document.cookie = "kandev_locale=pseudo; path=/; max-age=31536000; SameSite=Lax";
  });
  await testPage.reload();
  await expect(testPage.locator("html")).toHaveAttribute("lang", "pseudo");
  const titleInfo = testPage.locator(
    '[data-settings-target="setting-agent-generated-task-titles"] [data-settings-info]',
  );
  await titleInfo.tap();
  const drawer = testPage.getByRole("dialog");
  await expect(drawer).toBeVisible();
  const box = await drawer.boundingBox();
  expect(box!.x).toBeGreaterThanOrEqual(0);
  expect(box!.x + box!.width).toBeLessThanOrEqual(testPage.viewportSize()!.width);
  expect(
    await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
  ).toBe(true);
  await testPage.keyboard.press("Escape");
  await expect(titleInfo).toBeFocused();
});
