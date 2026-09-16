import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";

// @covers AC-UI-SIDEBAR-HOVER-002.1, AC-UI-SIDEBAR-HOVER-002.2, AC-UI-SIDEBAR-HOVER-002.3, AC-UI-SIDEBAR-HOVER-002.4
test("saves custom dwell and disabled hover without changing explicit expansion", async ({
  testPage,
}, testInfo) => {
  await testPage.goto("/settings/preferences/appearance");
  const toggle = testPage.getByRole("switch", { name: "Show sidebar on hover" });
  const delay = testPage.getByRole("spinbutton", { name: "Hover delay (ms)" });
  await expect(toggle).toBeChecked();
  await expect(delay).toHaveValue("500");
  await delay.fill("");
  await expect(testPage.getByRole("button", { name: "Save changes", exact: true })).toBeDisabled();
  await delay.fill("1200");
  const saved = waitForHttp(testPage, "PATCH", /\/api\/v1\/user\/settings$/);
  await testPage.getByRole("button", { name: "Save changes", exact: true }).click();
  expect((await saved).ok()).toBe(true);
  await expect(testPage.getByTestId("settings-floating-save")).toBeHidden();
  await testPage.reload();
  await expect(delay).toHaveValue("1200");
  await delay.scrollIntoViewIfNeeded();
  expect((await delay.boundingBox())!.height).toBe(28);
  await testPage.screenshot({ path: testInfo.outputPath("sidebar-hover-settings-desktop.png") });

  await testPage.goto("/");
  const sidebar = testPage.getByTestId("app-sidebar");
  await sidebar.getByRole("button", { name: "Collapse sidebar", exact: true }).click();
  await testPage.mouse.move(600, 400);
  await expect(sidebar).toHaveCSS("width", "56px");
  await testPage.clock.install();
  await testPage.clock.pauseAt(await testPage.evaluate(() => Date.now() + 100));
  await sidebar.hover({ position: { x: 20, y: 200 } });
  await testPage.clock.runFor(1199);
  await expect(sidebar).toHaveAttribute("data-hover-revealed", "false");
  await testPage.clock.runFor(1);
  await expect(sidebar).toHaveAttribute("data-hover-revealed", "true");
  await testPage.clock.resume();

  await testPage.goto("/settings/preferences/appearance");
  await toggle.click();
  await expect(delay).toBeDisabled();
  const disabled = waitForHttp(testPage, "PATCH", /\/api\/v1\/user\/settings$/);
  await testPage.getByRole("button", { name: "Save changes", exact: true }).click();
  expect((await disabled).ok()).toBe(true);
  await expect(testPage.getByTestId("settings-floating-save")).toBeHidden();
  await testPage.reload();
  await expect(toggle).not.toBeChecked();
  await expect(delay).toHaveValue("1200");
  await testPage.goto("/");
  await sidebar.getByRole("button", { name: "Collapse sidebar", exact: true }).click();
  await testPage.mouse.move(600, 400);
  await expect(sidebar).toHaveCSS("width", "56px");
  await testPage.clock.pauseAt(await testPage.evaluate(() => Date.now() + 100));
  await sidebar.hover({ position: { x: 20, y: 200 } });
  await testPage.clock.runFor(5000);
  await expect(sidebar).toHaveAttribute("data-hover-revealed", "false");
  await testPage.clock.resume();
  await sidebar.getByRole("button", { name: "Expand sidebar", exact: true }).click();
  await testPage.mouse.move(600, 400);
  await expect(sidebar).toHaveAttribute("data-collapsed", "false");
});
