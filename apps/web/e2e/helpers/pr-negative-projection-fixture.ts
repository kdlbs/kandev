import { expect, type Page } from "@playwright/test";
import type { SidebarTaskPageResponse, WorkflowSnapshot } from "@/lib/types/http";
import type { TaskPRsResponse } from "@/lib/types/github";
import type { TaskStatusSummary } from "@/lib/types/task-status-summary";

const COMPACT_SUMMARY_UPDATED_AT = "2099-01-01T00:00:00Z";
const FULL_PR_SYNCED_AT = "2098-12-31T00:00:00Z";

export async function installNewerNegativePRProjectionFixture(
  page: Page,
  taskId: string,
  prNumber: number,
) {
  let compactSummaryApplied = false;
  let fullPRSnapshotApplied = false;

  await page.addInitScript(
    ({ taskId, prNumber, updatedAt }) => {
      let boot: unknown;
      Object.defineProperty(window, "__KANDEV_BOOT_PAYLOAD__", {
        configurable: true,
        get: () => boot,
        set: (payload) => {
          const task = payload?.entities?.tasks?.[taskId];
          if (task) {
            const summary = task.status_summary ?? task.statusSummary ?? { revision: 1 };
            const projected = {
              ...summary,
              updated_at: updatedAt,
              pull_request: {
                ...(summary.pull_request ?? {}),
                number: prNumber,
                state: "merged",
                count: 1,
                workflow_approval_required: false,
              },
            };
            task.status_summary = projected;
            task.statusSummary = projected;
            (window as Window & { __negativePRBootApplied?: boolean }).__negativePRBootApplied =
              true;
          }
          boot = payload;
        },
      });
    },
    { taskId, prNumber, updatedAt: COMPACT_SUMMARY_UPDATED_AT },
  );

  await page.route("**/api/v1/workspaces/*/sidebar/query", async (route) => {
    const response = await route.fetch();
    const pageData = (await response.json()) as SidebarTaskPageResponse;
    for (const entry of pageData.entries) {
      if (entry.task?.id !== taskId) continue;
      entry.task.status_summary = {
        ...(entry.task.status_summary ?? { revision: 1 }),
        updated_at: COMPACT_SUMMARY_UPDATED_AT,
        pull_request: {
          number: prNumber,
          state: "merged",
          count: 1,
          workflow_approval_required: false,
        },
      };
      compactSummaryApplied = true;
    }
    await route.fulfill({ response, json: pageData });
  });

  await page.route("**/api/v1/workflows/*/snapshot", async (route) => {
    const response = await route.fetch();
    const snapshot = (await response.json()) as WorkflowSnapshot;
    const task = snapshot.tasks.find((candidate) => candidate.id === taskId);
    if (!task) {
      await route.fulfill({ response });
      return;
    }

    const summary: TaskStatusSummary = task.status_summary ?? {
      revision: 1,
      updated_at: COMPACT_SUMMARY_UPDATED_AT,
      pull_request: null,
    };
    task.status_summary = {
      ...summary,
      updated_at: COMPACT_SUMMARY_UPDATED_AT,
      pull_request: {
        ...(summary.pull_request ?? {}),
        number: prNumber,
        state: "merged",
        count: 1,
        workflow_approval_required: false,
      },
    };
    compactSummaryApplied = true;
    await route.fulfill({ response, json: snapshot });
  });

  await page.route("**/api/v1/github/task-prs?task_ids=*", async (route) => {
    const requestURL = new URL(route.request().url());
    if (requestURL.searchParams.get("task_ids") !== taskId) {
      await route.continue();
      return;
    }
    const response = await route.fetch();
    const pullRequests = (await response.json()) as TaskPRsResponse;
    const pr = pullRequests.task_prs[taskId]?.find((candidate) => candidate.pr_number === prNumber);
    if (!pr) {
      await route.fulfill({ response, json: pullRequests });
      return;
    }

    pr.last_synced_at = FULL_PR_SYNCED_AT;
    pr.workflow_attention = {
      state: "approval_required",
      head_sha: pr.head_sha ?? "negative-projection-head",
      observed_at: "2098-12-30T00:00:00Z",
      stale: true,
      runs: [],
    };
    fullPRSnapshotApplied = true;
    await route.fulfill({ response, json: pullRequests });
  });

  return async () => {
    await expect
      .poll(
        async () =>
          compactSummaryApplied ||
          (await page.evaluate(
            () =>
              (window as Window & { __negativePRBootApplied?: boolean }).__negativePRBootApplied ===
              true,
          )),
      )
      .toBe(true);
    await expect.poll(() => fullPRSnapshotApplied).toBe(true);

    expect(fullPRSnapshotApplied).toBe(true);
    expect(Date.parse(COMPACT_SUMMARY_UPDATED_AT)).toBeGreaterThan(Date.parse(FULL_PR_SYNCED_AT));
    expect(Date.parse(FULL_PR_SYNCED_AT)).toBeGreaterThan(Date.parse("2098-12-30T00:00:00Z"));
  };
}
