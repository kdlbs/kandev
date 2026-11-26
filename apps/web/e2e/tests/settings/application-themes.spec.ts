import { expect, test } from "../../fixtures/test-base";
import { importAndSelectTheme } from "./theme-flow";

test("imports, previews, saves and recovers an unavailable theme", async ({
  testPage,
  apiClient,
}, testInfo) => {
  test.setTimeout(120_000);
  const reference = await importAndSelectTheme(testPage);
  const id = reference.slice(6).split("/")[0];
  try {
    await testPage.screenshot({
      path: testInfo.outputPath("theme-appearance.png"),
      fullPage: true,
    });
    await apiClient.rawRequest("POST", `/api/plugins/${id}/disable`, {});
    await expect(
      testPage.getByRole("status").filter({ hasText: "Saved theme unavailable" }),
    ).toBeVisible();
    await expect
      .poll(() =>
        testPage.evaluate(() => document.documentElement.style.getPropertyValue("--background")),
      )
      .toBe("");
    expect(await testPage.evaluate(() => localStorage.getItem("theme"))).toBe(reference);
    await apiClient.rawRequest("POST", `/api/plugins/${id}/enable`, {});
    await expect
      .poll(() =>
        testPage.evaluate(() => document.documentElement.style.getPropertyValue("--background")),
      )
      .toBe("#272822");
  } finally {
    await apiClient.rawRequest("DELETE", `/api/plugins/${id}`).catch(() => undefined);
  }
});
