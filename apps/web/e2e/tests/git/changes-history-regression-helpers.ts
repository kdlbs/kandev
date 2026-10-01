import { expect, type Page } from "@playwright/test";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { AppState } from "../../../lib/state/store";
import { SessionPage } from "../../pages/session-page";
import { seedLargeWorkingTree } from "./large-changes-helpers";

const PROVIDER_BASE = "a".repeat(40);
const PROVIDER_HEAD = "b".repeat(40);
const LOCAL_HEAD = "c".repeat(40);
const PR_NUMBER = 950;

export const historySectionTestIds = [
  "unstaged-files-section-collapse-toggle",
  "pr-changes-section-collapse-toggle",
  "local-checkout-commits-section-collapse-toggle",
  "current-pr-commits-section-collapse-toggle",
];

export async function openHistoryRegression(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  mobile: boolean,
): Promise<SessionPage> {
  await api.mockGitHubReset();
  await api.mockGitHubAddPRs([
    {
      number: PR_NUMBER,
      title: "History regression",
      state: "open",
      head_branch: "main",
      base_branch: "main",
      author_login: "contributor",
      repo_owner: "testorg",
      repo_name: "testrepo",
      head_sha: PROVIDER_HEAD,
    },
  ]);
  await api.mockGitHubAddPRCommits("testorg", "testrepo", PR_NUMBER, [
    {
      sha: PROVIDER_BASE,
      message: "Published base",
      author_login: "contributor",
      author_date: "2026-10-01T10:00:00Z",
    },
    {
      sha: PROVIDER_HEAD,
      message: "Published head",
      author_login: "contributor",
      author_date: "2026-10-01T11:00:00Z",
    },
  ]);
  await api.mockGitHubAddPRFiles("testorg", "testrepo", PR_NUMBER, [
    { filename: "published.ts", status: "modified", additions: 1, deletions: 0 },
  ]);
  const task = await api.createTaskWithAgent(
    seed.workspaceId,
    "History regression",
    seed.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seed.workflowId,
      workflow_step_id: seed.startStepId,
      repository_ids: [seed.repositoryId],
    },
  );
  await api.mockGitHubAssociateTaskPR({
    task_id: task.id,
    repository_id: seed.repositoryId,
    owner: "testorg",
    repo: "testrepo",
    pr_number: PR_NUMBER,
    pr_url: `https://github.com/testorg/testrepo/pull/${PR_NUMBER}`,
    pr_title: "History regression",
    head_branch: "main",
    base_branch: "main",
    author_login: "contributor",
  });
  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForChatIdle();
  if (mobile) {
    await page
      .getByRole("navigation")
      .getByRole("button", { name: /Changes$/ })
      .click();
  } else {
    await session.clickTab("Changes");
  }
  await seedLargeWorkingTree(page, "flat", 1);
  return session;
}

export async function seedHistoryRelation(page: Page, kind: "stale" | "diverged" | "local_ahead") {
  await page.evaluate(
    ({ providerBase, providerHead, localHead, relationKind }) => {
      const store = (window as Window & { __KANDEV_E2E_STORE__?: { getState: () => AppState } })
        .__KANDEV_E2E_STORE__;
      if (!store) throw new Error("E2E store bridge missing");
      const state = store.getState();
      const sessionId = state.tasks.activeSessionId;
      const environmentId = sessionId && state.environmentIdBySessionId[sessionId];
      if (!sessionId || !environmentId) throw new Error("Active environment is unavailable");
      const filePath = "local-file-with-a-long-but-wrapped-mobile-name.ts";
      state.setGitStatus(environmentId, {
        branch: "main",
        remote_branch: "origin/main",
        head_commit: localHead,
        remote_head_commit: relationKind === "stale" ? providerBase : providerHead,
        remote_ahead: relationKind === "local_ahead" ? 1 : 2,
        remote_behind: relationKind === "diverged" ? 1 : 0,
        detail_state: "ready",
        modified: [],
        added: [],
        deleted: [],
        renamed: [],
        untracked: [filePath],
        ahead: 3,
        behind: 0,
        files: {
          [filePath]: {
            path: filePath,
            status: "untracked",
            staged: false,
            additions: 1,
            deletions: 0,
          },
        },
        timestamp: new Date(Date.now() + 7_200_000).toISOString(),
      });
      state.setSessionCommits(sessionId, [
        {
          id: "local-history",
          session_id: sessionId,
          commit_sha: localHead,
          parent_sha: providerHead,
          author_name: "Contributor",
          author_email: "contributor@example.invalid",
          commit_message: "Local checkout commit",
          committed_at: "2026-10-01T12:00:00Z",
          created_at: "2026-10-01T12:00:00Z",
          files_changed: 1,
          insertions: 1,
          deletions: 0,
          pushed: false,
        },
      ]);
    },
    {
      providerBase: PROVIDER_BASE,
      providerHead: PROVIDER_HEAD,
      localHead: LOCAL_HEAD,
      relationKind: kind,
    },
  );
}

export async function expectStaleHistory(page: Page) {
  await expect(page.getByTestId("commits-section-collapse-toggle")).toBeVisible();
  await expect(page.getByTestId("local-checkout-commits-section")).toHaveCount(0);
  await expect(page.getByTestId("current-pr-commits-section")).toHaveCount(0);
  await expect(page.getByTestId("header-remote-contribution-warning")).toHaveCount(0);
}

export async function expectDivergedHistory(page: Page) {
  await expect(page.getByTestId("local-checkout-commits-section-collapse-toggle")).toBeVisible();
  await expect(page.getByTestId("current-pr-commits-section-collapse-toggle")).toBeVisible();
  await expect(page.getByTestId("commits-section")).toHaveCount(0);
}

export async function expectHeaderGeometry(page: Page, expectedHeight: number, allowWrap = false) {
  for (const id of historySectionTestIds) {
    const toggle = page.getByTestId(id);
    await expect(toggle).toBeVisible();
    await expect
      .poll(async () => {
        const geometry = await toggle.evaluate((element) => {
          const control = element.getBoundingClientRect();
          const row = element
            .closest<HTMLElement>("[data-changes-timeline-row]")!
            .getBoundingClientRect();
          return {
            height: control.height,
            rowHeight: row.height,
            topOffset: control.top - row.top,
          };
        });
        return {
          controlHeight: allowWrap
            ? geometry.height >= expectedHeight - 1
            : Math.abs(geometry.height - expectedHeight) <= 1,
          wrapperMatchesControl: Math.abs(geometry.rowHeight - geometry.height) <= 1,
          controlStartsAtRow: Math.abs(geometry.topOffset) <= 1,
        };
      })
      .toEqual({ controlHeight: true, wrapperMatchesControl: true, controlStartsAtRow: true });
  }
}

export async function expectExpandedPRContiguous(page: Page) {
  await page.getByTestId("pr-changes-section-collapse-toggle").click();
  await expect(page.locator('[data-changes-file="published.ts"]')).toBeVisible();
  await expect
    .poll(() =>
      page.getByTestId("pr-changes-section-collapse-toggle").evaluate((element) => {
        const header = element.closest<HTMLElement>("[data-changes-timeline-row]")!;
        const section = header.closest<HTMLElement>('[data-testid="pr-files-section"]')!;
        const rows = section.querySelectorAll<HTMLElement>("[data-changes-timeline-row]");
        return rows[1].getBoundingClientRect().top - element.getBoundingClientRect().bottom;
      }),
    )
    .toBe(0);
  await page.getByTestId("pr-changes-section-collapse-toggle").click();
}
