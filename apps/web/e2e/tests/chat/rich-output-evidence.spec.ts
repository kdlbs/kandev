import { combinedReport, calloutLabels } from "./rich-output-evidence-helpers";
import { statusReport, statusItems, statusLabels } from "./rich-output-evidence-helpers";
import fs from "node:fs";
import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import {
  seedEvidenceTask,
  TABLE_FILE,
  tableReport,
  tableRows,
} from "./rich-output-evidence-helpers";

// @covers AC-AGENTS-AGENT-RICH-OUTPUT-003.1, AC-AGENTS-AGENT-RICH-OUTPUT-003.2, AC-AGENTS-AGENT-RICH-OUTPUT-003.3, AC-AGENTS-AGENT-RICH-OUTPUT-003.5
test("Table renders selected literal CSV cells and replays after source deletion", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  const session = await seedEvidenceTask(
    { page: testPage, apiClient, seedData, title: "Table evidence" },
    tableReport(),
  );
  const report = session.activeChat().getByTestId("rich-output");
  await expect(report).toContainText("Evidence Table report");
  const table = report.getByRole("table", { name: "CSV evidence" });
  await expect(table.getByRole("columnheader")).toHaveCount(8);
  await expect(table.getByRole("cell")).toHaveText(tableRows[0].map((value) => value || "Empty"));
  await expect(report.getByRole("table", { name: "Inline evidence" }).getByRole("cell")).toHaveText(
    tableRows[0].map((value) => value || "Empty"),
  );
  await expect(
    report.getByTestId("rich-output-table").filter({ hasText: "Empty evidence" }),
  ).toContainText("No rows");
  await expect(table.locator("script")).toHaveCount(0);
  await table.scrollIntoViewIfNeeded();
  await testPage.screenshot({ path: testInfo.outputPath("evidence-desktop.png") });
  fs.unlinkSync(path.join(seedData.repositoryPath, TABLE_FILE));
  fs.unlinkSync(path.join(seedData.repositoryPath, "empty-evidence.csv"));
  await testPage.reload();
  await session.waitForLoad();
  const persisted = session.activeChat().getByTestId("rich-output");
  await expect(persisted.getByRole("table", { name: "CSV evidence" }).getByRole("cell")).toHaveText(
    tableRows[0].map((value) => value || "Empty"),
  );
  await persisted.getByTestId("rich-output-file-preview-toggle").click();
  await expect(persisted).toContainText("RICH_OUTPUT_FILE_PREVIEW");
});

// @covers AC-AGENTS-AGENT-RICH-OUTPUT-003.6, AC-AGENTS-AGENT-RICH-OUTPUT-003.7
test("StatusList shows ordered static findings and does not change workflow state", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const session = await seedEvidenceTask(
    { page: testPage, apiClient, seedData, title: "StatusList findings" },
    statusReport(),
  );
  const report = session.activeChat().getByTestId("rich-output");
  const checks = report.getByRole("list", { name: "Verification" });
  const taskId = new URL(testPage.url()).pathname.split("/").at(-1)!;
  await expect
    .poll(async () => {
      const { sessions } = await apiClient.listTaskSessions(taskId);
      return sessions.every((entry) => entry.state === "WAITING_FOR_INPUT");
    })
    .toBe(true);
  const before = await apiClient.getTask(taskId);
  await expect(checks.locator("[data-status-label]")).toHaveText(statusLabels);
  await expect(checks.getByRole("listitem")).toHaveCount(5);
  for (const item of statusItems) await expect(checks).toContainText(item.detail);
  await expect(checks.locator("svg")).toHaveCount(5);
  await expect(checks.getByRole("button")).toHaveCount(0);
  await expect(
    report.getByTestId("rich-output-status-list").filter({ hasText: "Unrecorded checks" }),
  ).toContainText("No checks");
  await report.getByTestId("rich-output-file-preview-toggle").click();
  await expect(report).toContainText("RICH_OUTPUT_FILE_PREVIEW");
  const after = await apiClient.getTask(taskId);
  expect(after.workflow_step_id).toBe(before.workflow_step_id);
  expect(after.state).toBe(before.state);
  await expect(checks.locator("[data-status-label]")).toHaveText(statusLabels);
});

// @covers AC-AGENTS-AGENT-RICH-OUTPUT-003.8, AC-AGENTS-AGENT-RICH-OUTPUT-003.9
test("Callout mixed report retains chart filters and disclosure through transcript and viewport changes", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  const session = await seedEvidenceTask(
    { page: testPage, apiClient, seedData, title: "Callout findings" },
    combinedReport(),
  );
  const report = session.activeChat().getByTestId("rich-output");
  await expect(report.getByRole("note")).toHaveCount(4);
  await expect(report.locator("[data-callout-label]")).toHaveText(calloutLabels);
  await expect(report.getByRole("alert")).toHaveCount(0);
  await expect(report.locator("script")).toHaveCount(0);
  const chart = report.getByRole("figure", {
    name: "Service costs Monthly service costs and budget in USD",
  });
  await chart
    .getByTestId("rich-output-chart-plot")
    .evaluate((element) =>
      element.scrollIntoView({ block: "center", inline: "nearest", behavior: "instant" }),
    );
  const filter = report.getByTestId("rich-output-chart-legend-series_0");
  await filter.click();
  await expect(filter).toHaveAttribute("aria-pressed", "false");
  await report.getByTestId("rich-output-file-preview-toggle").click();
  await expect(report).toContainText("RICH_OUTPUT_FILE_PREVIEW");
  const taskId = new URL(testPage.url()).pathname.split("/").at(-1)!;
  const { sessions } = await apiClient.listTaskSessions(taskId);
  expect(sessions).toHaveLength(1);
  await expect
    .poll(async () => (await apiClient.listTaskSessions(taskId)).sessions[0].state)
    .toBe("WAITING_FOR_INPUT");
  await apiClient.seedSessionMessage(sessions[0].id, {
    type: "message",
    content: "Unrelated transcript update",
    authorType: "agent",
  });
  await expect(session.activeChat()).toContainText("Unrelated transcript update");
  for (const width of [390, 1280]) {
    await testPage.setViewportSize({ width, height: 900 });
    await expect(
      testPage.getByTestId(width < 768 ? "mobile-task-layout" : "dockview-task-layout"),
    ).toBeVisible();
    await expect(report).toContainText("RICH_OUTPUT_FILE_PREVIEW");
    const chatBounds = await session.activeChat().boundingBox();
    expect(chatBounds).not.toBeNull();
    await testPage.mouse.move(chatBounds!.x + chatBounds!.width / 2, chatBounds!.y + 100);
    await testPage.mouse.wheel(0, -350);
    await chart
      .getByTestId("rich-output-chart-plot")
      .evaluate((element) =>
        element.scrollIntoView({ block: "center", inline: "nearest", behavior: "instant" }),
      );
    await expect(filter).toHaveAttribute("aria-pressed", "false");
    await expect(report).toContainText("RICH_OUTPUT_FILE_PREVIEW");
    await expect(report.getByRole("note")).toHaveCount(4);
  }
  await report.getByRole("note").first().scrollIntoViewIfNeeded();
  await testPage.screenshot({ path: testInfo.outputPath("callout-desktop.png") });
});
