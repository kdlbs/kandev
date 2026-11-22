import fs from "node:fs";
import type { Page } from "@playwright/test";
import { expect } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import { firstSessionId } from "../../helpers/session";
import type { SeedData } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

const MONITOR_IDS = ["monitor-task-first", "monitor-task-second"] as const;
const MONITOR_PROMPT = "/claude-monitor-lifecycle";
const DRAFT = "keep this draft while both Monitors finish";

async function createClaudeMonitorProfile(
  apiClient: ApiClient,
  configDir: string,
  signalFile: string,
): Promise<string> {
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

  const profile = await apiClient.createAgentProfile(agentId, `Claude Monitor E2E ${Date.now()}`, {
    model: "mock-fast",
    env_vars: [
      { key: "CLAUDE_CONFIG_DIR", value: configDir },
      { key: "E2E_MOCK_AGENT_CLAUDE_MONITOR_SIGNAL_FILE", value: signalFile },
    ],
  });
  return profile.id;
}

async function runningMonitorCount(apiClient: ApiClient, sessionId: string): Promise<number> {
  const { workloads } = await apiClient.listBackgroundWorkloads(sessionId);
  return workloads.filter((workload) => workload.kind === "monitor" && workload.state === "running")
    .length;
}

async function expectMonitorState(
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

async function expectMonitorCardStatus(
  session: SessionPage,
  monitorIndex: number,
  status: "watching" | "ended",
): Promise<void> {
  const cards = session.activeChat().locator('[data-testid="monitor-card"]:visible');
  await expect(cards).toHaveCount(2, { timeout: 30_000 });
  await expect(cards.nth(monitorIndex)).toContainText(status);
}

async function expandMonitorCards(session: SessionPage): Promise<void> {
  const cards = session.activeChat().locator('[data-testid="monitor-card"]:visible');
  if ((await cards.count()) === 2) return;
  const groups = session.activeChat().getByRole("button", { name: /2 tool calls/ });
  const groupCount = await groups.count();
  for (let groupIndex = groupCount - 1; groupIndex >= 0; groupIndex--) {
    const group = groups.nth(groupIndex);
    await expect(group).toBeVisible({ timeout: 30_000 });
    if ((await group.getAttribute("aria-expanded")) !== "true") {
      await group.click();
    }
    if ((await cards.count()) === 2) return;
  }
  await expect(cards).toHaveCount(2, { timeout: 30_000 });
}

async function expectMonitorLabels(page: Page): Promise<void> {
  const summary = page.getByTestId("background-work-summary-list");
  await expect(summary).toBeVisible();
  for (const workId of MONITOR_IDS) {
    const item = summary.getByTestId(`background-work-summary-item-${workId}`);
    await expect(item).toBeVisible();
    await expect(item).toContainText("Monitor");
  }
}

async function closeMonitorSummary(page: Page): Promise<void> {
  if ((page.viewportSize()?.width ?? 0) < 768) {
    await page.getByTestId("background-work-summary-close").tap();
  } else {
    await page.keyboard.press("Escape");
  }
  await expect(page.getByTestId("background-work-summary-list")).toBeHidden();
}

function agentSessionAddPanelButton(page: Page, sessionID: string) {
  return page
    .locator(".dv-tabs-and-actions-container")
    .filter({ has: page.getByTestId(`session-tab-${sessionID}`) })
    .getByTestId("dockview-add-panel-btn");
}

async function openHistory(page: Page, sessionID: string): Promise<void> {
  if ((page.viewportSize()?.width ?? 0) < 768) {
    await page.getByTestId("mobile-session-nav-panels").tap();
    await page.getByTestId("mobile-background-work-open").tap();
    await expect(page.getByTestId("background-work-mobile-panel")).toBeVisible();
  } else {
    const addPanelButton = agentSessionAddPanelButton(page, sessionID);
    await expect(addPanelButton).toHaveCount(1);
    await addPanelButton.click();
    await page.getByTestId("add-panel-background-work").click();
    await expect(page.getByTestId("background-work-overview")).toBeVisible();
  }
  await expect(page.getByTestId("background-work-tab-history")).toHaveAttribute(
    "data-state",
    "active",
  );
}

/**
 * Exercises the managed Claude completion evidence with two live Monitors.
 * The mock appends each native notification only after the test sees both
 * registrations, and keeps the second Monitor live while the first settles.
 */
export async function expectClaudeMonitorLifecycle(
  page: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  configDir: string,
  signalFile: string,
): Promise<void> {
  fs.mkdirSync(configDir, { recursive: true });
  fs.rmSync(signalFile, { force: true });
  const profileId = await createClaudeMonitorProfile(apiClient, configDir, signalFile);
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Claude Monitor lifecycle",
    profileId,
    {
      description: "Claude Monitor lifecycle fixture",
      repository_ids: [seedData.repositoryId],
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    },
  );

  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  const sessionId = task.session_id ?? (await firstSessionId(apiClient, task.id));

  await session.sendMessageViaButton(MONITOR_PROMPT);
  await session.expectChatResponseVisible("Two Claude Monitors are watching.");
  await session.waitForChatIdle({ timeout: 30_000 });
  await expect
    .poll(() => runningMonitorCount(apiClient, sessionId), {
      message: "both Monitor registrations should be projected as running",
      timeout: 30_000,
    })
    .toBe(2);
  // Wait for the transcript to settle the turn before opening its auto-expanded tool group.
  const monitorPrompt = session
    .activeChat()
    .locator('[id^="msg-"]')
    .filter({
      has: page.getByTestId("user-message-bubble").getByText(MONITOR_PROMPT, { exact: true }),
    });
  await expect(monitorPrompt.getByTestId("message-turn-duration")).toHaveCount(1);
  await expandMonitorCards(session);
  const monitorCards = session.activeChat().locator('[data-testid="monitor-card"]:visible');
  await expectMonitorCardStatus(session, 0, "watching");
  await expectMonitorCardStatus(session, 1, "watching");

  const chip = page.getByTestId("background-work-chip");
  await expect(chip).toBeVisible();
  await expect(chip).toContainText("2 Background Jobs");
  if ((page.viewportSize()?.width ?? 0) < 768) {
    await chip.tap();
  } else {
    await chip.click();
  }
  await expectMonitorLabels(page);
  await closeMonitorSummary(page);

  // Reload while both native watches are active, then hand off the foreground
  // turn to a successor prompt. Neither transition ends the Monitor runs.
  await page.reload();
  await session.waitForLoad();
  await expect
    .poll(() => runningMonitorCount(apiClient, sessionId), {
      message: "both Monitors should remain live after reload",
      timeout: 30_000,
    })
    .toBe(2);
  await expandMonitorCards(session);
  await expect(monitorCards).toHaveCount(2, { timeout: 30_000 });
  await expectMonitorCardStatus(session, 0, "watching");
  await expectMonitorCardStatus(session, 1, "watching");
  await expect(chip).toBeVisible();
  await expect(chip).toContainText("2 Background Jobs");
  if ((page.viewportSize()?.width ?? 0) < 768) {
    await chip.tap();
  } else {
    await chip.click();
  }
  await expectMonitorLabels(page);
  await closeMonitorSummary(page);

  await session.sendMessageViaButton("Continue while the Monitors watch.");
  await session.expectChatResponseVisible("I've completed the analysis of your request");
  await session.waitForChatIdle({ timeout: 30_000 });
  await expect
    .poll(() => runningMonitorCount(apiClient, sessionId), {
      message: "the successor prompt should leave both Monitors live",
      timeout: 30_000,
    })
    .toBe(2);

  const editor = await session.fillMessage(DRAFT);
  fs.writeFileSync(signalFile, "complete-first\n");
  await expectMonitorState(apiClient, sessionId, MONITOR_IDS[0], "completed");
  await expect
    .poll(() => runningMonitorCount(apiClient, sessionId), {
      message: "the first completion should leave its sibling running",
      timeout: 30_000,
    })
    .toBe(1);
  await expectMonitorCardStatus(session, 0, "ended");
  await expectMonitorCardStatus(session, 1, "watching");
  await expect(chip).toBeVisible();
  await expect(chip).toContainText("1 Background Job");
  await expect(chip).not.toContainText("2 Background Jobs");

  await expect(editor).toHaveText(DRAFT);
  await page.reload();
  await session.waitForLoad();
  await expectMonitorState(apiClient, sessionId, MONITOR_IDS[0], "completed");
  await expect
    .poll(() => runningMonitorCount(apiClient, sessionId), {
      message: "the sibling Monitor should stay live after reload",
      timeout: 30_000,
    })
    .toBe(1);
  await expandMonitorCards(session);
  await expect(monitorCards).toHaveCount(2, { timeout: 30_000 });
  await expectMonitorCardStatus(session, 0, "ended");
  await expectMonitorCardStatus(session, 1, "watching");
  await expect(page.getByTestId("background-work-chip")).toContainText("1 Background Job");
  fs.appendFileSync(signalFile, "complete-second\n");
  await expectMonitorState(apiClient, sessionId, MONITOR_IDS[1], "completed");
  await expect
    .poll(() => runningMonitorCount(apiClient, sessionId), {
      message: "the last completion should clear the running Monitor projection",
      timeout: 30_000,
    })
    .toBe(0);
  await expect(chip).toHaveCount(0);
  await expectMonitorCardStatus(session, 0, "ended");
  await expectMonitorCardStatus(session, 1, "ended");

  await openHistory(page, sessionId);
  for (const workId of MONITOR_IDS) {
    await expect(page.getByTestId(`background-workload-card-${workId}`)).toBeVisible();
    await expect(page.getByTestId(`background-work-state-${workId}`)).toHaveText("Completed");
  }

  await page.reload();
  if ((page.viewportSize()?.width ?? 0) < 768) {
    await expect(page.getByTestId("mobile-session-nav-panels")).toBeVisible({ timeout: 30_000 });
  } else {
    const addPanelButton = agentSessionAddPanelButton(page, sessionId);
    await expect(addPanelButton).toHaveCount(1, { timeout: 30_000 });
    await expect(addPanelButton).toBeVisible();
  }
  await openHistory(page, sessionId);
  for (const workId of MONITOR_IDS) {
    await expect(page.getByTestId(`background-workload-card-${workId}`)).toBeVisible();
    await expect(page.getByTestId(`background-work-state-${workId}`)).toHaveText("Completed");
  }
}
