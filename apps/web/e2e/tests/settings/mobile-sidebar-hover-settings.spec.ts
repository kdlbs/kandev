import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";

// @covers AC-UI-SIDEBAR-HOVER-002.3, AC-UI-SIDEBAR-HOVER-002.5
test("phone can save both hover preferences with contained touch controls", async ({
  testPage,
}, testInfo) => {
  await testPage.goto("/settings/preferences/appearance");
  const card = testPage.getByTestId("sidebar-hover-settings-card");
  const toggle = card.getByRole("switch", { name: "Show sidebar on hover" });
  const delay = card.getByRole("spinbutton", { name: "Hover delay (ms)" });
  await card.scrollIntoViewIfNeeded();
  for (const control of [toggle, delay]) {
    const bounds = await control.boundingBox();
    expect(bounds!.height).toBeGreaterThanOrEqual(44);
    expect(bounds!.width).toBeGreaterThanOrEqual(44);
  }
  await delay.fill("900");
  await toggle.tap();
  await expect(delay).toBeDisabled();
  const saved = waitForHttp(testPage, "PATCH", /\/api\/v1\/user\/settings$/);
  await testPage.getByRole("button", { name: "Save changes", exact: true }).tap();
  expect((await saved).ok()).toBe(true);
  await expect(testPage.getByTestId("settings-floating-save")).toBeHidden();
  await testPage.reload();
  await expect(toggle).not.toBeChecked();
  await expect(delay).toHaveValue("900");
  await card.scrollIntoViewIfNeeded();
  expect(
    await testPage.evaluate(
      () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
    ),
  ).toBe(0);
  await testPage.screenshot({ path: testInfo.outputPath("sidebar-hover-settings-phone.png") });
  await testPage.goto("/");
  await expect(testPage.getByTestId("app-sidebar")).toBeHidden();
});
