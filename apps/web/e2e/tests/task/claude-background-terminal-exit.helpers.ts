import { expect, type Locator, type Page } from "@playwright/test";
import fs from "node:fs";
import type { ApiClient } from "../../helpers/api-client";
import { firstSessionId } from "../../helpers/session";
import type { SeedData } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

const FIRST_JOB_TITLE = "Run the full test suite";
const SECOND_JOB_TITLE = "Watch pull request checks";

type TerminalJobs = { first: string; second: string };

async function resolveTerminalJobs(apiClient: ApiClient, sessionId: string): Promise<TerminalJobs> {
  const { workloads } = await apiClient.listBackgroundWorkloads(sessionId);
  const first = workloads.find((workload) => workload.title === FIRST_JOB_TITLE)?.work_id ?? "";
  const second = workloads.find((workload) => workload.title === SECOND_JOB_TITLE)?.work_id ?? "";
  expect(first).not.toBe("");
  expect(second).not.toBe("");
  return { first, second };
}

async function createClaudeACpProfile(apiClient: ApiClient): Promise<string> {
  let agentId = "";
  await expect
    .poll(
      async () => {
        const { agents } = await apiClient.listAgents();
        agentId = agents.find((agent) => agent.name === "claude-acp")?.id ?? "";
        return agentId;
      },
      { message: "claude-acp agent should be registered", timeout: 30_000 },
    )
    .not.toBe("");

  const profile = await apiClient.createAgentProfile(agentId, `Claude terminal E2E ${Date.now()}`, {
    model: "mock-fast",
  });
  return profile.id;
}

async function openBackgroundWorkHistory(page: Page): Promise<void> {
  if ((page.viewportSize()?.width ?? 0) < 768) {
    await page.getByTestId("mobile-session-nav-panels").click();
    await page.getByTestId("mobile-background-work-open").tap();
    await expect(page.getByTestId("background-work-mobile-panel")).toBeVisible();
  } else {
    await page.getByTestId("dockview-add-panel-btn").first().click();
    await page.getByTestId("add-panel-background-work").click();
    await expect(page.getByTestId("background-work-overview")).toBeVisible();
  }
  await expect(page.getByTestId("background-work-tab-history")).toHaveAttribute(
    "data-state",
    "active",
  );
}

async function expectJobState(
  apiClient: ApiClient,
  sessionId: string,
  workId: string,
  state: string,
): Promise<void> {
  await expect
    .poll(
      async () => {
        const { workloads } = await apiClient.listBackgroundWorkloads(sessionId);
        return workloads.find((workload) => workload.work_id === workId)?.state ?? "";
      },
      { message: `${workId} should reach ${state}`, timeout: 30_000 },
    )
    .toBe(state);
}

async function expectRunningCount(
  apiClient: ApiClient,
  sessionId: string,
  count: number,
): Promise<void> {
  await expect
    .poll(
      async () => {
        const { workloads } = await apiClient.listBackgroundWorkloads(sessionId);
        return workloads.filter((workload) => workload.state === "running").length;
      },
      { message: `background running count should be ${count}`, timeout: 30_000 },
    )
    .toBe(count);
}

/**
 * A qualified Claude 0.97.0 terminal frame ends only its matching detached
 * shell with a null outcome. Desktop and phone share this flow through their
 * own specs (REQ-AGENTS-BACKGROUND-WORK-005 and -010, AC .5 and .6).
 * `signalFile` is the test-owned gate that releases the fixture's terminal
 * frames only after both running jobs have been observed.
 */
export async function expectClaudeBackgroundTerminalExit(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  signalFile: string,
): Promise<void> {
  const agentProfileId = await createClaudeACpProfile(apiClient);
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Claude background terminal exit",
    agentProfileId,
    {
      description: "Claude background terminal",
      repository_ids: [seedData.repositoryId],
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    },
  );

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  const sessionId = task.session_id ?? (await firstSessionId(apiClient, task.id));

  // Start the fixture only after the page is ready so both launch frames are
  // observable before the test releases the terminal frames.
  await session.sendMessageViaButton("/claude-background-terminal 15s 8s");
  await session.expectChatResponseVisible("Two background commands are running.");

  await expectRunningCount(apiClient, sessionId, 2);
  const jobs = await resolveTerminalJobs(apiClient, sessionId);
  const { workloads: running } = await apiClient.listBackgroundWorkloads(sessionId);
  const launchOrigin =
    running.find((workload) => workload.work_id === jobs.first)?.origin_turn_id ?? "";
  expect(launchOrigin).not.toBe("");
  const chip = testPage.getByTestId("background-work-chip");
  await expect(chip).toBeVisible();
  await expect(chip).toContainText("2 Background Jobs");

  const editor: Locator = await session.fillMessage("draft survives Claude terminal exits");
  fs.writeFileSync(signalFile, "release-terminal-frames");

  await expectJobState(apiClient, sessionId, jobs.first, "ended");
  await expectRunningCount(apiClient, sessionId, 1);
  await expect(chip).toContainText("1 Background Job");
  await expect(chip).not.toContainText("2 Background Jobs");

  const { workloads: afterFirst } = await apiClient.listBackgroundWorkloads(sessionId);
  const first = afterFirst.find((workload) => workload.work_id === jobs.first);
  expect(first?.exit_code).toBeUndefined();
  expect(first?.source_call_id).toBe(jobs.first);
  expect(first?.origin_turn_id).toBe(launchOrigin);

  await expectJobState(apiClient, sessionId, jobs.second, "ended");
  await expectRunningCount(apiClient, sessionId, 0);
  await expect(chip).toHaveCount(0);
  await expect(editor).toHaveText("draft survives Claude terminal exits");

  await openBackgroundWorkHistory(testPage);
  for (const workId of [jobs.first, jobs.second]) {
    await expect(testPage.getByTestId(`background-workload-card-${workId}`)).toBeVisible();
    await expect(testPage.getByTestId(`background-work-state-${workId}`)).toHaveText("Ended");
  }

  // Persisted rows survive a full reload; the consumed terminal frames cannot
  // be replayed, so this proves the durable projection rather than the socket.
  await testPage.reload();
  if ((testPage.viewportSize()?.width ?? 0) < 768) {
    await expect(testPage.getByTestId("mobile-session-nav-panels")).toBeVisible({
      timeout: 30_000,
    });
  } else {
    await expect(testPage.getByTestId("dockview-add-panel-btn").first()).toBeVisible({
      timeout: 30_000,
    });
  }
  await openBackgroundWorkHistory(testPage);
  for (const workId of [jobs.first, jobs.second]) {
    await expect(testPage.getByTestId(`background-workload-card-${workId}`)).toBeVisible();
  }
}
