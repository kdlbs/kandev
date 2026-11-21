import { expect, test, type Page } from "@playwright/test";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import { installFixturePlugin, uninstallFixturePlugin } from "./plugin-fixture";
import { PrAssetCapture } from "./pr-asset-capture";
import {
  expectUsageCopy,
  readUsage,
  reportFixtureUsage,
  selectUsageView,
  usageControl,
  usageTarget,
  waitForUsage,
  type UsageFixtureObservation,
} from "./usage-history";

async function createUsageTask(api: ApiClient, seed: SeedData, title: string) {
  return api.createTaskWithAgent(seed.workspaceId, title, seed.agentProfileId, {
    workflow_id: seed.workflowId,
    workflow_step_id: seed.startStepId,
    repository_ids: [seed.repositoryId],
    executor_profile_id: seed.worktreeExecutorProfileId,
    start_agent: false,
    prepare_session: true,
  });
}

export async function exerciseCollectorRetention(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  touch: boolean,
) {
  const capture = new PrAssetCapture(page, test.info().file, {
    captureKey: touch ? "phone-real-collector" : "desktop-real-collector",
  });
  await installFixturePlugin(page);
  try {
    const task = await createUsageTask(api, seed, "Collector accounting lifecycle");
    const target = await usageTarget(page, seed.workspaceId, task.id);
    const item: UsageFixtureObservation = {
      subject_binding: target.SubjectBinding,
      item_id: "retained-large",
      revision: 1,
      model_id: "exact-fixture-model",
      agent_client: "client-is-not-a-provider",
      coverage: { OccurrenceAt: new Date().toISOString() },
      tokens: { input_tokens: "9007199254740993", total_tokens: "9007199254740993" },
      quality: { TokenCoverageComplete: true, CostCoverageComplete: false },
    };
    await reportFixtureUsage(page, seed.workspaceId, item);
    await waitForUsage(page, seed.workspaceId, "9007199254740993");
    await api.archiveTask(task.id);
    await page.goto(`/stats/token-usage?workspaceId=${seed.workspaceId}&range=all`);
    let breakdown = await selectUsageView(page, "Tasks", touch);
    const archivedRow = breakdown
      .getByTestId(touch ? "token-usage-mobile-row" : "token-usage-row")
      .filter({ hasText: task.id });
    await expect(archivedRow).toHaveCount(1);
    await expect(archivedRow.getByText("Archived", { exact: true })).toBeVisible();
    const disclosure = archivedRow.locator("summary");
    if (touch) {
      await disclosure.tap();
      await expect(
        archivedRow
          .getByTestId("token-usage-details")
          .getByRole("link", { name: task.id, exact: true }),
      ).toHaveAttribute("href", `/t/${task.id}`);
    } else {
      await expect(breakdown.getByRole("link", { name: task.id, exact: true })).toHaveAttribute(
        "href",
        `/t/${task.id}`,
      );
    }
    if (capture.capturing) {
      if (!touch) await disclosure.click();
      const details = archivedRow.getByTestId("token-usage-details");
      await expect(details).toContainText("client-is-not-a-provider");
      await archivedRow.scrollIntoViewIfNeeded();
      await capture.screenshot("real-collector-archived-details");
    }
    await api.deleteTask(task.id);
    const deleted = await readUsage(page, seed.workspaceId);
    expect(deleted.rows).toHaveLength(1);
    expect(deleted.rows[0].task_available).toBe(false);
    expect(deleted.totals.total_tokens).toBeNull();
    expect(deleted.totals.amounts_decimal?.total_tokens).toBe("9007199254740993");
    await reportFixtureUsage(page, seed.workspaceId, {
      ...item,
      revision: 2,
      tokens: { input_tokens: "9007199254740995", total_tokens: "9007199254740995" },
    });
    const corrected = await waitForUsage(page, seed.workspaceId, "9007199254740995");
    expect(corrected.rows[0].provider ?? "").toBe("");
    expect(corrected.rows[0].agent_client).toBe("client-is-not-a-provider");
    await page.reload();
    breakdown = await selectUsageView(page, "Tasks", touch);
    const row = breakdown.getByTestId(touch ? "token-usage-mobile-row" : "token-usage-row");
    await expect(row).toContainText("Deleted task");
    await expect(row).toContainText("9,007,199,254,740,995");
    await expect(row.locator("a")).toHaveCount(0);
    const details = row.locator("summary");
    if (touch) await details.tap();
    else await details.click();
    await expect(row.getByTestId("token-usage-details")).toContainText("client-is-not-a-provider");
    await expect(row.getByTestId("token-usage-details")).toContainText("Unknown");
    await expectUsageCopy(page, "9,007,199,254,740,995", touch);
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);
    await row.scrollIntoViewIfNeeded();
    await capture.screenshot("real-collector-deleted-details");
  } finally {
    capture.flush();
    await uninstallFixturePlugin(api);
  }
}

export async function exerciseProjectionRecovery(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  fault: boolean,
) {
  await installFixturePlugin(page);
  const workspace = seed.workspaceId;
  try {
    await usageControl(page, workspace, "pause");
    // Stop before creating the task so the empty workspace generation cannot publish.
    const task = await createUsageTask(api, seed, "Real projection preparation and rebuild");
    const target = await usageTarget(page, workspace, task.id);
    const item: UsageFixtureObservation = {
      subject_binding: target.SubjectBinding,
      item_id: "rebuild-correction",
      revision: 1,
      model_id: "rebuild-model",
      provider: "fixture-provider",
      coverage: { OccurrenceAt: new Date().toISOString() },
      tokens: { input_tokens: "41", total_tokens: "41" },
      quality: { TokenCoverageComplete: true, CostCoverageComplete: false },
    };
    await reportFixtureUsage(page, workspace, item);
    const first = await readUsage(page, workspace);
    expect(first.projection_status).toBe("preparing");
    expect(first.projection_generation).toBe("");
    expect(first.rows).toEqual([]);
    await page.goto(`/stats/token-usage?workspaceId=${workspace}&range=all`);
    await expect(page.getByTestId("token-usage-preparing")).toContainText(
      "Preparing usage history.",
    );
    await expect(page.getByTestId("token-usage-summary")).toHaveCount(0);
    await usageControl(page, workspace, "resume");
    const complete = await waitForUsage(page, workspace, "41");
    await page.getByRole("button", { name: "Retry status", exact: true }).click();
    await expect(page.getByTestId("token-usage-summary")).toContainText("41");
    await usageControl(page, workspace, "pause");
    await usageControl(page, workspace, "rebuild");
    await reportFixtureUsage(page, workspace, {
      ...item,
      revision: 2,
      tokens: { input_tokens: "43", total_tokens: "43" },
    });
    const pending = await readUsage(page, workspace);
    expect(pending.projection_generation).toBe(complete.projection_generation);
    expect(pending.projection_pending).toBe(true);
    expect(pending.totals.amounts_decimal?.total_tokens).toBe("41");
    await page.reload();
    await expect(page.getByTestId("token-usage-projection-notice")).toContainText(
      "Saved values remain visible. Updating history...",
    );
    await expect(page.getByTestId("token-usage-summary")).toContainText("41");
    if (fault) await exerciseRealUsageReadFault(page, workspace);
    await usageControl(page, workspace, "resume");
    // The real publication read also proves persistence recovered after the injected deadline.
    const next = await waitForUsage(page, workspace, "43");
    expect(next.projection_generation).not.toBe(complete.projection_generation);
    if (fault) {
      await expect(page.getByTestId("token-usage-error")).toHaveCount(2);
      await expect(page.getByTestId("token-usage-summary")).toContainText("41");
      await page
        .getByTestId("token-usage-error")
        .first()
        .getByRole("button", { name: "Retry", exact: true })
        .click();
    } else {
      await page.getByRole("button", { name: "Retry status", exact: true }).click();
    }
    await expect(page.getByTestId("token-usage-summary")).toContainText("43");
    await expect(page.getByTestId("token-usage-error")).toHaveCount(0);
    await expect(page.getByTestId("token-usage-preparing")).toHaveCount(0);
    await expect(page.getByTestId("token-usage-projection-notice")).toHaveCount(0);
  } finally {
    try {
      await usageControl(page, workspace, "release-reader");
    } finally {
      try {
        await usageControl(page, workspace, "resume");
      } finally {
        await uninstallFixturePlugin(api);
      }
    }
  }
}

async function exerciseRealUsageReadFault(page: Page, workspace: string) {
  await usageControl(page, workspace, "hold-reader");
  const failed = page.waitForResponse(
    (response) =>
      /\/stats\/token-usage$/.test(new URL(response.url()).pathname) && response.status() === 503,
  );
  await page.getByRole("button", { name: "Retry status", exact: true }).click();
  expect((await (await failed).json()).error_code).toBe("analytics_busy");
  await expect(page.getByTestId("token-usage-error").first()).toBeVisible();
  await expect(page.getByTestId("token-usage-error")).toHaveCount(2);
  await expect(page.getByTestId("token-usage-error").nth(1)).toBeVisible();
  await expect(page.getByTestId("token-usage-summary")).toContainText("41");
  await usageControl(page, workspace, "release-reader");
  await expect(page.getByTestId("token-usage-error")).toHaveCount(2);
  await expect(page.getByTestId("token-usage-summary")).toContainText("41");
  await expect(page.getByTestId("token-usage-projection-notice")).toContainText(
    "Saved values remain visible. Updating history...",
  );
}
