import { expect, type Page } from "@playwright/test";
import type { TokenUsageResponse } from "../../lib/types/http";
import { PLUGIN_ID } from "./plugin-fixture";

export type UsageTarget = {
  SubjectBinding: string;
  TaskID: string;
  SessionID: string | null;
  ActorAttributionStatus: string;
};

type UsageTargetPage = {
  Targets: UsageTarget[];
  SnapshotID: string;
  NextCursor: string;
  HasMore: boolean;
};

export type UsageFixtureObservation = {
  subject_binding: string;
  item_id: string;
  revision: number;
  model_id?: string;
  provider?: string;
  agent_client?: string;
  work_identity?: { Scheme: string; ID: string };
  coverage?: {
    OccurrenceAt?: string;
    CoverageStart?: string;
    CoverageEnd?: string;
    SourceDate?: string;
    SourceTimezone?: string;
  };
  tokens?: Record<string, string>;
  cost?: { amount: string | null; currency: string; basis: "reported" | "estimated" | "unknown" };
  quality?: { TokenCoverageComplete: boolean; CostCoverageComplete: boolean };
};

export async function usageFixture<T>(page: Page, workspace: string, body: object): Promise<T> {
  const response = await page.request.post(
    `/api/plugins/${PLUGIN_ID}/actions/usage-history-fixture`,
    {
      data: { workspaceId: workspace, body },
    },
  );
  expect(response.status(), await response.text()).toBe(200);
  return response.json() as Promise<T>;
}

export async function usageTarget(
  page: Page,
  workspace: string,
  task: string,
  session?: string,
): Promise<UsageTarget> {
  let snapshot = "";
  let cursor = "";
  for (let pageIndex = 0; pageIndex < 20; pageIndex += 1) {
    const result = await usageFixture<UsageTargetPage>(page, workspace, {
      operation: "targets",
      snapshot_id: snapshot,
      cursor,
    });
    const target = result.Targets.find(
      (item) => item.TaskID === task && (!session || item.SessionID === session),
    );
    if (target) return target;
    if (!result.HasMore) break;
    expect(result.NextCursor).not.toBe("");
    snapshot = result.SnapshotID;
    cursor = result.NextCursor;
  }
  throw new Error(
    `The real Host did not issue a collection target for ${task}/${session ?? "task"}`,
  );
}

export async function reportFixtureUsage(
  page: Page,
  workspace: string,
  observation: UsageFixtureObservation,
) {
  const result = await usageFixture<{
    CollectorNamespace: string;
    Results: { Status: string; Error: string }[];
  }>(page, workspace, {
    operation: "report",
    ...observation,
  });
  expect(result.CollectorNamespace).toBe(`plugin:${PLUGIN_ID}`);
  expect(result.Results).toHaveLength(1);
  expect(result.Results[0].Status, result.Results[0].Error).toBe("accepted");
}

export async function readUsage(page: Page, workspace: string, query: Record<string, string> = {}) {
  const params = new URLSearchParams({ range: "all", group: "task", timezone: "UTC", ...query });
  const response = await page.request.get(
    `/api/v1/workspaces/${workspace}/stats/token-usage?${params}`,
  );
  expect(response.status(), await response.text()).toBe(200);
  return response.json() as Promise<TokenUsageResponse>;
}

export async function waitForUsage(
  page: Page,
  workspace: string,
  tokens: string,
  query: Record<string, string> = {},
) {
  let report: TokenUsageResponse | undefined;
  await expect
    .poll(
      async () => {
        report = await readUsage(page, workspace, query);
        return {
          status: report.projection_status,
          pending: report.projection_pending,
          published: Boolean(report.projection_generation),
          tokens: report.totals.amounts_decimal?.total_tokens,
        };
      },
      { timeout: 30_000, message: "Waiting for the real retained publication and exact totals" },
    )
    .toEqual({
      status: "ready",
      pending: false,
      published: true,
      tokens,
    });
  if (!report) throw new Error("No retained report was read");
  return report;
}

export async function usageControl(
  page: Page,
  workspace: string,
  action: "pause" | "resume" | "rebuild" | "hold-reader" | "release-reader",
) {
  const response = await page.request.post(`/api/v1/e2e/usage-history/${workspace}/${action}`);
  expect(response.status(), await response.text()).toBe(action === "rebuild" ? 202 : 200);
}

export async function selectUsageView(page: Page, view: "Tasks" | "Sessions", touch: boolean) {
  const breakdown = page.getByTestId("token-usage-breakdown");
  if (touch) {
    await breakdown.getByRole("combobox", { name: "View", exact: true }).tap();
    await page.getByRole("option", { name: view, exact: true }).tap();
  } else {
    await breakdown.getByRole("tab", { name: view, exact: true }).click();
  }
  return breakdown;
}

export async function expectUsageCopy(page: Page, expected: string, touch: boolean) {
  await page.evaluate(() =>
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: {
        writeText: async (text: string) => {
          (window as Window & { usageCopy?: string }).usageCopy = text;
        },
      },
    }),
  );
  const copy = page.getByRole("button", { name: "Copy Stats", exact: true });
  if (touch) await copy.tap();
  else await copy.click();
  await expect
    .poll(() => page.evaluate(() => (window as Window & { usageCopy?: string }).usageCopy))
    .toContain(`Total tokens: ${expected}`);
}
