import { expect, test, type Page } from "@playwright/test";
import type { BackendContext } from "../fixtures/backend";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import type { TokenUsageResponse } from "../../lib/types/http";
import { pruneSavedDelivery } from "./delivery-retention";
import {
  readUsage,
  reportFixtureUsage,
  selectUsageView,
  usageFixture,
  usageTarget,
  waitForUsage,
} from "./usage-history";
import { installFixturePlugin, PLUGIN_ID, uninstallFixturePlugin } from "./plugin-fixture";
import { PrAssetCapture } from "./pr-asset-capture";
import {
  activateRetention,
  analyzeRetention,
  readRetentionMessage,
  resetRetention,
  RETENTION_ROUTE,
  runRetention,
  seedRetentionTask,
  withRetentionPolicyRestore,
} from "./tool-payload-retention";

export async function exerciseNativeRetention(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  backend: BackendContext,
  touch: boolean,
) {
  const capture = new PrAssetCapture(page, test.info().file, {
    captureKey: touch ? "phone-real-native" : "desktop-real-native",
  });
  let before: TokenUsageResponse | undefined;
  const native = await pruneSavedDelivery(page, api, seed, backend, {
    prompt: "/slow 1s /with-usage",
    beforePrune: async () => {
      before = await waitForUsage(page, seed.workspaceId, "150");
    },
  });
  const pruned = await waitForUsage(page, seed.workspaceId, "150");
  expect(pruned.totals).toEqual(before?.totals);
  expect(pruned.totals.token_source?.source).toBe("native");
  await installFixturePlugin(page);
  try {
    const withCost = await attachReportedNativeCost(
      page,
      seed.workspaceId,
      native.task.id,
      native.sessionId,
    );
    await compactToolPayloads(page, api, seed, backend, { touch, before: withCost });
    await api.archiveTask(native.task.id);
    await api.deleteSession(native.sessionId);
    const retained = await readUsage(page, seed.workspaceId, { group: "session" });
    expect(retained.totals.amounts_decimal?.total_tokens).toBe("150");
    expect(retained.totals.amounts_decimal?.cost_subcents).toBe("1200");
    expect(retained.rows).toHaveLength(1);
    expect(retained.rows[0].session_available).toBe(false);
    await page.goto(`/stats/token-usage?workspaceId=${seed.workspaceId}&range=all`);
    const breakdown = await selectUsageView(page, "Sessions", touch);
    const row = breakdown.getByTestId(touch ? "token-usage-mobile-row" : "token-usage-row");
    await expect(row).toContainText("Unavailable session");
    await expect(row.locator(`a[href*="${native.sessionId}"]`)).toHaveCount(0);
    await expect(page.getByTestId("token-usage-summary")).toContainText("150");
    const disclosure = row.locator("summary");
    if (touch) await disclosure.tap();
    else await disclosure.click();
    await expect(row.getByTestId("token-usage-details")).toBeVisible();
    await row.scrollIntoViewIfNeeded();
    await capture.screenshot("real-native-retained-unavailable-session-details");
  } finally {
    capture.flush();
    await uninstallFixturePlugin(api);
  }
}

async function attachReportedNativeCost(
  page: Page,
  workspace: string,
  task: string,
  session: string,
) {
  const canonical = await usageFixture<{
    rows: { SourceRecordID: string; ObservedAt: string; Model: string; Provider: string }[];
  }>(page, workspace, {
    operation: "canonical",
    filter: { TaskIDs: [task], SessionIDs: [session] },
  });
  expect(canonical.rows).toHaveLength(1);
  const native = canonical.rows[0];
  const target = await usageTarget(page, workspace, task, session);
  await reportFixtureUsage(page, workspace, {
    subject_binding: target.SubjectBinding,
    item_id: "reported-native-cost",
    revision: 1,
    model_id: native.Model,
    ...(native.Provider && { provider: native.Provider }),
    work_identity: { Scheme: "provider_event", ID: native.SourceRecordID },
    coverage: { OccurrenceAt: native.ObservedAt },
    tokens: { input_tokens: "120", output_tokens: "30", total_tokens: "150" },
    cost: { amount: "1200", currency: "USD", basis: "reported" },
    quality: { TokenCoverageComplete: false, CostCoverageComplete: true },
  });
  const report = await waitForUsage(page, workspace, "150");
  expect(report.totals.amounts_decimal?.cost_subcents).toBe("1200");
  expect(report.totals.token_source?.source).toBe("native");
  expect(report.totals.cost_source?.source).toBe(`plugin:${PLUGIN_ID}`);
  expect(report.totals.cost_basis).toBe("reported");
  return report;
}

async function compactToolPayloads(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  backend: BackendContext,
  options: { touch: boolean; before: TokenUsageResponse },
) {
  const { touch, before } = options;
  await withRetentionPolicyRestore(page, async () => {
    await resetRetention(page);
    const payload = await seedRetentionTask(backend.tmpDir, api, seed, false);
    expect(
      readRetentionMessage(backend.tmpDir, payload.messageId).metadata.normalized.shell_exec.output,
    ).toHaveProperty("stdout");
    await page.goto(RETENTION_ROUTE);
    await analyzeRetention(page, touch);
    await activateRetention(page, "skip", touch);
    await runRetention(page, touch);
    expect(
      readRetentionMessage(backend.tmpDir, payload.messageId).metadata.normalized.shell_exec.output,
    ).not.toHaveProperty("stdout");
    const compacted = await waitForUsage(page, seed.workspaceId, "150");
    expect(compacted.totals).toEqual(before.totals);
  });
}
