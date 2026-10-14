import { combinedReport, calloutLabels } from "./rich-output-evidence-helpers";
import { statusReport, statusItems, statusLabels } from "./rich-output-evidence-helpers";
import { test, expect } from "../../fixtures/test-base";
import {
  seedEvidenceTask,
  tableReport,
  tableColumns,
  tableRows,
} from "./rich-output-evidence-helpers";

// @covers AC-AGENTS-AGENT-RICH-OUTPUT-003.5, AC-AGENTS-AGENT-RICH-OUTPUT-003.9
test("Table phone rows retain every labelled field and neighboring preview state", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  const session = await seedEvidenceTask(
    { page: testPage, apiClient, seedData, title: "Phone Table evidence" },
    tableReport(),
  );
  const report = session.activeChat().getByTestId("rich-output");
  const csv = report
    .getByTestId("rich-output-table")
    .filter({ has: testPage.getByRole("heading", { name: "CSV evidence", exact: true }) });
  await expect(csv.locator("dt")).toHaveText(tableColumns.map((c) => c.label));
  await expect(csv.locator("dd")).toHaveText(tableRows[0].map((value) => value || "Empty"));
  await expect(csv.getByRole("table")).toHaveCount(0);
  await csv.scrollIntoViewIfNeeded();
  await testPage.screenshot({ path: testInfo.outputPath("evidence-phone.png") });
  await report.getByTestId("rich-output-file-preview-toggle").tap();
  await expect(report).toContainText("RICH_OUTPUT_FILE_PREVIEW");
  for (const width of [767, 768, 390]) {
    await testPage.setViewportSize({ width, height: 900 });
    await expect(csv.locator(width < 768 ? "dt" : "th")).toHaveText(
      tableColumns.map((c) => c.label),
    );
    await expect(report).toContainText("RICH_OUTPUT_FILE_PREVIEW");
    await expect
      .poll(() =>
        testPage.evaluate(
          () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
        ),
      )
      .toBe(true);
  }
});

// @covers AC-AGENTS-AGENT-RICH-OUTPUT-003.6, AC-AGENTS-AGENT-RICH-OUTPUT-003.9
test("StatusList phone findings wrap with visible static labels and an empty state", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  const session = await seedEvidenceTask(
    { page: testPage, apiClient, seedData, title: "Phone StatusList" },
    statusReport(),
  );
  const report = session.activeChat().getByTestId("rich-output");
  const checks = report.getByRole("list", { name: "Verification" });
  await expect(checks.locator("[data-status-label]")).toHaveText(statusLabels);
  for (const item of statusItems) await expect(checks).toContainText(item.detail);
  await expect(
    report.getByTestId("rich-output-status-list").filter({ hasText: "Unrecorded checks" }),
  ).toContainText("No checks");
  await checks.scrollIntoViewIfNeeded();
  await testPage.screenshot({ path: testInfo.outputPath("status-list-phone.png") });
  await expect
    .poll(() =>
      testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    )
    .toBe(true);
  await expect(checks.getByRole("button")).toHaveCount(0);
});

// @covers AC-AGENTS-AGENT-RICH-OUTPUT-003.8, AC-AGENTS-AGENT-RICH-OUTPUT-003.9
test("Callout phone report contains all static note kinds and literal text", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  const session = await seedEvidenceTask(
    { page: testPage, apiClient, seedData, title: "Phone Callout" },
    combinedReport(),
  );
  const report = session.activeChat().getByTestId("rich-output");
  await expect(report.getByRole("note")).toHaveCount(4);
  await expect(report.locator("[data-callout-label]")).toHaveText(calloutLabels);
  for (const note of await report.getByRole("note").all()) {
    await expect(note).toContainText("<script>literal note</script>");
    await expect(note.locator("svg")).toHaveCount(1);
  }
  await expect(report.getByRole("alert")).toHaveCount(0);
  await expect(report.locator("script")).toHaveCount(0);
  await report.getByRole("note").first().scrollIntoViewIfNeeded();
  await testPage.screenshot({ path: testInfo.outputPath("callout-phone.png") });
  await expect
    .poll(() =>
      testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    )
    .toBe(true);
});
