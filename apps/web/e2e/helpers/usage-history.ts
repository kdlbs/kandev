import { expect, type Page } from "@playwright/test";
import type { TokenUsageResponse } from "../../lib/types/http";
import { PLUGIN_ID } from "./plugin-fixture";

export type UsageTarget = {
  SubjectBinding: string;
  TaskID: string;
  SessionID: string | null;
  ActorAttributionStatus: string;
  SourceScopeStatus: string;
};

type UsageTargetPage = {
  Targets: UsageTarget[] | null;
  Preparing: boolean;
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
  const state: { target?: UsageTarget; failed: boolean; error?: unknown } = { failed: false };
  const message = `The real Host did not issue an eligible collection target for ${task}/${session ?? "task"}`;
  await expect
    .poll(
      async () => {
        try {
          state.target = await findUsageTarget(page, workspace, task, session);
        } catch (error) {
          state.failed = true;
          state.error = error;
        }
        return state.failed || state.target !== undefined;
      },
      { message },
    )
    .toBe(true);
  if (state.failed) throw state.error;
  if (!state.target) throw new Error(message);
  return state.target;
}

async function findUsageTarget(
  page: Page,
  workspace: string,
  task: string,
  session?: string,
): Promise<UsageTarget | undefined> {
  let snapshot = "";
  let cursor = "";
  for (let pageIndex = 0; pageIndex < 20; pageIndex += 1) {
    const result = await usageFixture<UsageTargetPage>(page, workspace, {
      operation: "targets",
      snapshot_id: snapshot,
      cursor,
    });
    if (result.Preparing) return undefined;
    const target = (result.Targets ?? []).find(
      (item) =>
        item.SourceScopeStatus === "eligible" &&
        item.TaskID === task &&
        (!session || item.SessionID === session),
    );
    if (target) return target;
    if (!result.HasMore) return undefined;
    expect(result.NextCursor).not.toBe("");
    snapshot = result.SnapshotID;
    cursor = result.NextCursor;
  }
  throw new Error("The bounded Host target snapshot did not terminate");
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
  const response = await page.request.get(usageReadURL(workspace, query));
  expect(response.status(), await response.text()).toBe(200);
  return response.json() as Promise<TokenUsageResponse>;
}

function usageReadURL(workspace: string, query: Record<string, string>) {
  const params = new URLSearchParams({ range: "all", group: "task", timezone: "UTC", ...query });
  return `/api/v1/workspaces/${workspace}/stats/token-usage?${params}`;
}

function isTransientUsageReadFailure(status: number, responseBody: string) {
  if (status !== 503) return false;

  let body: unknown;
  try {
    body = JSON.parse(responseBody);
  } catch {
    return false;
  }

  if (typeof body !== "object" || body === null) return false;
  const payload = body as { code?: unknown; error_code?: unknown };
  return [payload.code, payload.error_code].some(
    (code) => code === "analytics_busy" || code === "persistence_unavailable",
  );
}

export async function waitForUsage(
  page: Page,
  workspace: string,
  tokens: string,
  query: Record<string, string> = {},
) {
  let report: TokenUsageResponse | undefined;
  let terminalError: Error | undefined;
  await expect
    .poll(
      async () => {
        const response = await page.request.get(usageReadURL(workspace, query));
        if (response.status() !== 200) {
          const responseBody = await response.text();
          if (isTransientUsageReadFailure(response.status(), responseBody)) return false;
          terminalError = new Error(
            `${responseBody}: expected HTTP 200, received ${response.status()}`,
          );
          return true;
        }
        report = (await response.json()) as TokenUsageResponse;
        return (
          report.projection_status === "ready" &&
          !report.projection_pending &&
          Boolean(report.projection_generation) &&
          report.totals.amounts_decimal?.total_tokens === tokens
        );
      },
      { timeout: 30_000, message: "Waiting for the real retained publication and exact totals" },
    )
    .toBe(true);
  if (terminalError) throw terminalError;
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
