import { expect, test } from "../../fixtures/test-base";
import { retainedReport, retainedRow } from "../../../app/stats/token-usage.test-helpers";
import {
  exerciseCollectorRetention,
  exerciseProjectionRecovery,
} from "../../helpers/usage-history-browser";
import { exerciseNativeRetention } from "../../helpers/usage-history-native";

test.describe("Phone retained usage presentation", () => {
  test.describe.configure({ retries: 0 });
  test("exposes exact retained amounts and actor details with touch controls, focus and contained scrolling", async ({
    testPage,
    seedData,
  }) => {
    const deleted = retainedRow({
      total_tokens: null,
      input_tokens: null,
      amounts_decimal: { total_tokens: "9007199254740993", input_tokens: "9007199254740961" },
    });
    await testPage.route("**/api/v1/workspaces/*/stats/token-usage**", (route) =>
      route.fulfill({ json: retainedReport({ rows: [deleted], totals: deleted }) }),
    );
    await testPage.goto(`/stats/token-usage?workspaceId=${seedData.workspaceId}`);
    const breakdown = testPage.getByTestId("token-usage-breakdown");
    await breakdown.getByRole("combobox", { name: "View", exact: true }).tap();
    await testPage.getByRole("option", { name: "Tasks", exact: true }).tap();
    const row = breakdown.getByTestId("token-usage-mobile-row");
    const summary = row.locator("summary");
    await expect(summary).toContainText("Deleted task");
    await expect(summary).toContainText("9,007,199,254,740,993");
    await summary.tap();
    await expect(row).toHaveAttribute("open");
    const details = row.getByTestId("token-usage-details");
    for (const text of [
      "Mixed users",
      "Unknown",
      "exact-model-id",
      "agent-client-a",
      "Asia/Kathmandu",
      "Source calendar. Exact rebucketing is unavailable.",
    ]) {
      await expect(details.getByText(text, { exact: true }).first()).toBeVisible();
    }
    await expect(row.locator("a")).toHaveCount(0);
    for (const control of [
      summary,
      breakdown.getByRole("combobox", { name: "Sort", exact: true }),
      breakdown.getByRole("button", { name: "Next page", exact: true }),
    ]) {
      const box = await control.boundingBox();
      if (!box) throw new Error("Retained usage touch control is not rendered");
      expect(box.height).toBeGreaterThanOrEqual(44);
    }
    await summary.focus();
    await testPage.keyboard.press("Enter");
    await expect(row).not.toHaveAttribute("open");
    await expect(summary).toBeFocused();
    await testPage.evaluate(() =>
      Object.defineProperty(navigator, "clipboard", {
        configurable: true,
        value: {
          writeText: async (text: string) => {
            (window as Window & { usageCopy?: string }).usageCopy = text;
          },
        },
      }),
    );
    await testPage.getByRole("button", { name: "Copy Stats", exact: true }).tap();
    await expect
      .poll(() => testPage.evaluate(() => (window as Window & { usageCopy?: string }).usageCopy))
      .toContain("Total tokens: 9,007,199,254,740,993");
    expect(await testPage.evaluate(() => document.documentElement.scrollWidth)).toBe(
      await testPage.evaluate(() => document.documentElement.clientWidth),
    );
    await expect(testPage.getByTestId("token-usage-scroll-body")).toHaveCSS("overflow-y", "auto");
  });
});

test("phone: real collector retention and correction keep totals and details usable", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  await exerciseCollectorRetention(testPage, apiClient, seedData, true);
});

test("phone: real preparing history and a later rebuild keep complete values", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  await exerciseProjectionRecovery(testPage, apiClient, seedData, false);
});

test("phone: native retained totals survive pruning and unavailable sessions have no broken link", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}) => {
  test.setTimeout(180_000);
  await exerciseNativeRetention(testPage, apiClient, seedData, backend, true);
});
