import { expect, test } from "../../fixtures/test-base";
import { seedRichOutputFailureMessage, seedRichOutputTask } from "./rich-output-helpers";

// @covers AC-AGENTS-AGENT-RICH-OUTPUT-001.22, AC-AGENTS-AGENT-RICH-OUTPUT-001.23
test("opens the retained rich-output error in a contained phone drawer", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}, testInfo) => {
  test.skip(testInfo.project.name !== "mobile-chrome", "phone-only rich-output disclosure");
  const session = await seedRichOutputTask({
    page: testPage,
    apiClient,
    seedData,
    title: "Mobile rich output error disclosure",
    failureRetry: true,
  });
  const chat = session.activeChat();
  const report = chat.getByTestId("rich-output");
  const trigger = chat.getByTestId("rich-output-error-trigger");

  await expect(trigger).toBeVisible();
  await expect(report.getByRole("heading", { name: "Release health" })).toBeVisible();
  await expect(report).toContainText("Corrected verification");
  await trigger.tap();
  const drawer = testPage.getByTestId("rich-output-error-drawer");
  await expect(drawer).toBeVisible();
  const dialog = testPage.getByRole("dialog", { name: "Visual output error" });
  await expect(dialog).toBeVisible();
  const descriptionId = await dialog.getAttribute("aria-describedby");
  expect(descriptionId).toBeTruthy();
  await expect(testPage.getByTestId("rich-output-error-scroll-region")).toHaveAttribute(
    "id",
    descriptionId!,
  );
  await expect(drawer).toContainText("StatusList");
  await expect(report.locator("h3")).toHaveText("Release health");
  await expect(report).toContainText("Corrected verification");

  const triggerBounds = await trigger.boundingBox();
  const close = drawer.getByRole("button", { name: "Close", exact: true });
  const closeBounds = await close.boundingBox();
  expect(triggerBounds?.width).toBeGreaterThanOrEqual(44);
  expect(triggerBounds?.height).toBeGreaterThanOrEqual(44);
  expect(closeBounds?.width).toBeGreaterThanOrEqual(44);
  expect(closeBounds?.height).toBeGreaterThanOrEqual(44);
  await expect(drawer.locator(".overflow-y-auto")).toHaveCount(1);

  const viewport = testPage.viewportSize()!;
  await expect
    .poll(async () => {
      const bounds = await drawer.boundingBox();
      return bounds !== null && bounds.y + bounds.height <= viewport.height;
    })
    .toBe(true);
  const drawerBounds = await drawer.boundingBox();
  expect(drawerBounds).not.toBeNull();
  expect(drawerBounds!.x).toBeGreaterThanOrEqual(0);
  expect(drawerBounds!.y).toBeGreaterThanOrEqual(0);
  expect(drawerBounds!.x + drawerBounds!.width).toBeLessThanOrEqual(viewport.width);
  expect(drawerBounds!.y + drawerBounds!.height).toBeLessThanOrEqual(viewport.height);
  expect(
    await testPage.evaluate(
      () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
    ),
  ).toBe(true);
  await testPage.screenshot({ path: testInfo.outputPath("rich-output-error-phone.png") });
  await prCapture.screenshot("phone-error-drawer", {
    caption: "Phone rich-output error disclosure",
  });

  await close.tap();
  await expect(drawer).toBeHidden();
  await expect(trigger).toBeFocused();
  await testPage.reload();
  await session.waitForLoad();
  const replayedTrigger = session.activeChat().getByTestId("rich-output-error-trigger");
  await expect(replayedTrigger).toHaveCount(1);
  await expect(session.activeChat().getByTestId("rich-output")).toContainText(
    "Corrected verification",
  );
  await replayedTrigger.tap();
  await expect(drawer).toBeVisible();
});

// @covers AC-AGENTS-AGENT-RICH-OUTPUT-001.22
test("wraps and scrolls a long retained diagnostic inside the phone drawer", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  test.skip(testInfo.project.name !== "mobile-chrome", "phone-only rich-output disclosure");
  const session = await seedRichOutputTask({
    page: testPage,
    apiClient,
    seedData,
    title: "Long rich output error disclosure",
  });
  const taskId = new URL(testPage.url()).pathname.split("/").at(-1)!;
  const { sessions } = await apiClient.listTaskSessions(taskId);
  const diagnostic = Array.from(
    { length: 90 },
    (_, index) => `Retained validation detail ${index + 1}: ${"long-value-".repeat(12)}`,
  ).join("\n");
  await seedRichOutputFailureMessage(apiClient, sessions[0].id, diagnostic);
  await testPage.reload();
  await session.waitForLoad();

  const trigger = session.activeChat().getByTestId("rich-output-error-trigger").last();
  await trigger.tap();
  const drawer = testPage.getByTestId("rich-output-error-drawer");
  const scrollRegion = drawer.getByTestId("rich-output-error-scroll-region");
  await expect(scrollRegion).toContainText("Retained validation detail 90");
  await expect(drawer.locator(".overflow-y-auto")).toHaveCount(1);
  const geometry = await scrollRegion.evaluate((element) => ({
    clientHeight: element.clientHeight,
    scrollHeight: element.scrollHeight,
    overflowY: getComputedStyle(element).overflowY,
  }));
  expect(geometry.scrollHeight).toBeGreaterThan(geometry.clientHeight);
  expect(["auto", "scroll"]).toContain(geometry.overflowY);
  await scrollRegion.evaluate((element) => element.scrollTo({ top: element.scrollHeight }));
  await expect(scrollRegion).toContainText("Retained validation detail 90");
  await testPage.screenshot({ path: testInfo.outputPath("rich-output-error-phone-long.png") });
});
