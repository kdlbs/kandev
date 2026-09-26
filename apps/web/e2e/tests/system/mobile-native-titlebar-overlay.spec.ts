import { expect, test } from "../../fixtures/test-base";

test("mobile browser keeps its normal page chrome without a native titlebar overlay", async ({
  testPage,
}) => {
  await testPage.goto("/settings/system/status");

  const shell = testPage.getByTestId("app-shell");
  await expect(shell).toHaveAttribute("data-window-controls-overlay", "hidden");
  await expect(shell).not.toHaveAttribute("data-macos-tauri-overlay", "true");
  await expect(testPage.getByTestId("app-sidebar")).toBeHidden();
  expect(
    await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
  ).toBe(true);
});
