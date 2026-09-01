import { execSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";

import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { waitForSessionState } from "../../helpers/session";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";
import type { Page } from "@playwright/test";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Create a passthrough (TUI) agent profile for the mock agent. */
async function createTUIProfile(apiClient: ApiClient, name: string) {
  const { agents } = await apiClient.listAgents();
  return apiClient.createAgentProfile(agents[0].id, name, {
    model: "mock-fast",
    auto_approve: true,
    cli_passthrough: true,
  });
}

/** Navigate to a kanban card by title and open its session page. */
async function openTaskSession(page: Page, title: string): Promise<SessionPage> {
  const kanban = new KanbanPage(page);
  await kanban.goto();

  const card = kanban.taskCardByTitle(title);
  await expect(card).toBeVisible({ timeout: 15_000 });
  await card.click();
  await expect(page).toHaveURL(/\/t\//, { timeout: 15_000 });

  const session = new SessionPage(page);
  await session.waitForLoad();
  return session;
}

type SessionTabHistoryEntry = { id: string; text: string };

async function recordSessionTabHistory(page: Page): Promise<void> {
  await page.addInitScript(() => {
    const history: SessionTabHistoryEntry[][] = [];
    const record = () => {
      const entries = Array.from(
        document.querySelectorAll<HTMLElement>('[data-testid^="session-tab-"]'),
      )
        .map((element) => {
          const testId = element.getAttribute("data-testid") ?? "";
          return {
            id: testId.slice("session-tab-".length),
            text: element.textContent?.trim() ?? "",
          };
        })
        .filter((entry) => entry.id && entry.text);
      if (entries.length > 0) history.push(entries);
    };
    const observer = new MutationObserver(record);
    const start = () => {
      observer.observe(document.documentElement, {
        subtree: true,
        childList: true,
        characterData: true,
      });
      record();
    };
    if (document.documentElement) {
      start();
    } else {
      document.addEventListener("DOMContentLoaded", start, { once: true });
    }
    (
      window as Window & { __KANDEV_SESSION_TAB_HISTORY__?: SessionTabHistoryEntry[][] }
    ).__KANDEV_SESSION_TAB_HISTORY__ = history;
  });
}

// ---------------------------------------------------------------------------
// Tests — ACP (normal) mode
// ---------------------------------------------------------------------------

test.describe("Session resume (ACP mode)", () => {
  // These tests restart the backend mid-test, which can be flaky
  test.describe.configure({ retries: 1 });

  test("resume after backend restart preserves messages and accepts new prompts", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);

    // 1. Create task and start agent with a simple scenario
    await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Resume ACP Task",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    // 2. Navigate to the session and wait for agent to finish its first turn
    const session = await openTaskSession(testPage, "Resume ACP Task");
    await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 30_000,
    });
    await session.waitForChatIdle({ timeout: 15_000 });

    // 3. Verify the "Started agent Mock" boot message appeared on initial launch
    await expect(session.chat.getByText("Started agent Mock", { exact: false })).toBeVisible({
      timeout: 15_000,
    });

    // 4. Restart the backend — kills the process, respawns with same DB/config
    await backend.restart();

    // 5. Reload the page so SSR fetches from the new backend instance
    await testPage.reload();
    await session.waitForLoad();

    // 6. Previous messages should still be visible (loaded from DB via SSR)
    await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 15_000,
    });
    await expect(session.chat.getByText("/e2e:simple-message")).toBeVisible({ timeout: 15_000 });

    // 7. Wait for auto-resume to complete — useSessionResumption hook detects
    //    needs_resume=true and relaunches the agent via session.launch.
    //    The full cycle (backend restart → health check → page reload → SSR →
    //    WS reconnect → auto-resume → agent turn) can be slow under CI load.
    await session.waitForChatIdle({ timeout: 60_000 });

    // 8. Verify the "Resumed agent Mock" boot message appeared after resume
    await expect(session.chat.getByText("Resumed agent Mock", { exact: false })).toBeVisible({
      timeout: 15_000,
    });

    // 9. Send a follow-up message to verify the agent works after resume
    await session.sendMessage("/e2e:simple-message");

    // 10. The agent should respond to the new prompt
    await session.expectChatResponseVisible("simple mock response", 1, { timeout: 30_000 });
  });

  // @covers AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-002.2
  test("preserves the worktree Files path after backend restart", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Worktree Path Resume Task",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.worktreeExecutorProfileId,
      },
    );
    const sessionId = task.session_id;
    if (!sessionId) throw new Error("createTaskWithAgent did not return a session_id");

    const session = await openTaskSession(testPage, "Worktree Path Resume Task");
    await session.waitForChatIdle({ timeout: 30_000 });
    await waitForSessionState(apiClient, {
      taskId: task.id,
      sessionId,
      expectedState: "WAITING_FOR_INPUT",
      message: "Initial worktree session did not settle before restart",
      timeout: 30_000,
    });

    const { sessions } = await apiClient.listTaskSessions(task.id);
    const initialSession = sessions.find((candidate) => candidate.id === sessionId);
    const expectedWorkspacePath = initialSession?.workspace_path ?? initialSession?.worktree_path;
    if (!expectedWorkspacePath) throw new Error("Worktree session did not expose a workspace path");
    const expectedDisplayPath = expectedWorkspacePath.replace(/^\/(?:Users|home)\/[^/]+\//, "~/");

    await session.clickTab("Files");
    const visibleWorkspacePath = session.files.getByTestId("file-browser-workspace-path");
    await expect(visibleWorkspacePath).toHaveText(expectedDisplayPath, { timeout: 30_000 });

    // Keeping Files active makes reload reconstruct the workspace-only execution
    // before automatic session resume promotes and persists that execution.
    await backend.restart();
    await testPage.reload();
    await expect(session.files).toBeVisible({ timeout: 30_000 });

    await session.clickSessionChatTab();
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 60_000 });
    await expect(session.chat.getByText("Resumed agent Mock", { exact: false })).toBeVisible({
      timeout: 15_000,
    });

    // Reload once promotion has persisted the task environment. The boot payload
    // must project the promoted execution's canonical workspace, not stale UI state.
    await testPage.reload();
    await session.waitForLoad();
    await session.clickTab("Files");
    await expect(visibleWorkspacePath).toHaveText(expectedDisplayPath, { timeout: 30_000 });
  });
});

// ---------------------------------------------------------------------------
// Tests — task status preservation during resume
// ---------------------------------------------------------------------------

test.describe("Task status during resume", () => {
  test.describe.configure({ retries: 1 });

  test("task stays in Turn Finished section after backend restart and agent resume", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);

    // 1. Create task and start agent — after the turn completes the workflow
    //    advances it from "Running" to "Turn Finished".
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Status Stable Task",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    const sessionId = task.session_id;
    if (!sessionId) throw new Error("createTaskWithAgent did not return a session_id");

    // 2. Navigate to the session and wait for the agent to finish its first turn
    const session = await openTaskSession(testPage, "Status Stable Task");
    await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 30_000,
    });
    await session.waitForChatIdle({ timeout: 15_000 });
    await waitForSessionState(apiClient, {
      taskId: task.id,
      sessionId,
      expectedState: "WAITING_FOR_INPUT",
      message: "Initial session did not settle before the Turn Finished assertion",
      timeout: 30_000,
    });
    await expect
      .poll(async () => (await apiClient.getTask(task.id)).state, {
        timeout: 30_000,
        message: "Initial task did not advance to REVIEW before the Turn Finished assertion",
      })
      .toBe("REVIEW");

    // 3. Confirm the task moved to the "Turn Finished" section after the turn completed
    await expect(session.taskInSection("Status Stable Task", "Turn Finished")).toBeVisible({
      timeout: 30_000,
    });

    // 4. Restart the backend
    await backend.restart();

    // 5. Reload the page so SSR fetches from the new backend instance
    await testPage.reload();
    await session.waitForLoad();

    // 6. Immediately after reload, the task must still be in "Turn Finished" — not
    //    regressed to "Backlog" or "Running" due to resume lifecycle.
    await expect
      .poll(async () => (await apiClient.getTask(task.id)).state, {
        timeout: 30_000,
        message: "Task state regressed while reloading before auto-resume",
      })
      .toBe("REVIEW");
    await expect(session.taskInSection("Status Stable Task", "Turn Finished")).toBeVisible({
      timeout: 30_000,
    });
    await expect(session.taskInSection("Status Stable Task", "Running")).not.toBeVisible({
      timeout: 5_000,
    });
    await expect(session.taskInSection("Status Stable Task", "Backlog")).not.toBeVisible({
      timeout: 5_000,
    });

    // 7. Wait for auto-resume to complete (agent relaunches and becomes idle)
    await session.waitForChatIdle({ timeout: 60_000 });
    // The idle composer can briefly survive the reload before auto-resume starts.
    // The durable boot message proves that the resumed agent actually relaunched.
    await expect(session.chat.getByText("Resumed agent Mock", { exact: false })).toBeVisible({
      timeout: 15_000,
    });
    await waitForSessionState(apiClient, {
      taskId: task.id,
      sessionId,
      expectedState: "WAITING_FOR_INPUT",
      message: "Resumed session did not settle before the final Turn Finished assertion",
      timeout: 30_000,
    });

    // 8. After resume completes, the task must still be in "Turn Finished"
    await expect
      .poll(async () => (await apiClient.getTask(task.id)).state, {
        timeout: 30_000,
        message: "Task state regressed after auto-resume",
      })
      .toBe("REVIEW");
    await expect(session.taskInSection("Status Stable Task", "Turn Finished")).toBeVisible({
      timeout: 30_000,
    });
  });
});

// ---------------------------------------------------------------------------
// Tests — TUI (passthrough) mode
// ---------------------------------------------------------------------------

test.describe("Session resume (TUI passthrough mode)", () => {
  test.describe.configure({ retries: 1 });

  test("resume TUI session after backend restart reconnects with resume flag", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);

    // 1. Create a TUI agent profile
    const tuiProfile = await createTUIProfile(apiClient, "TUI Resume");

    // 2. Create task with TUI agent
    await apiClient.createTaskWithAgent(seedData.workspaceId, "TUI Resume Task", tuiProfile.id, {
      description: "hello from resume test",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    // 3. Navigate and wait for TUI terminal to load
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const card = kanban.taskCardByTitle("TUI Resume Task");
    await expect(card).toBeVisible({ timeout: 15_000 });
    await card.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForPassthroughLoad();
    await session.waitForPassthroughLoaded();

    // 4. Verify initial TUI header
    await session.expectPassthroughHasText("Mock Agent");

    // 5. Wait for the workflow step to advance (idle timeout fires turn complete)
    await expect(session.stepperStep("Review")).toHaveAttribute("aria-current", "step", {
      timeout: 30_000,
    });

    // 6. Restart the backend
    await backend.restart();

    // 7. Reload the page — forces SSR re-fetch and WS reconnect
    await testPage.reload();

    // 8. Wait for passthrough terminal to reconnect after resume
    await session.waitForPassthroughLoad();
    await session.waitForPassthroughLoaded();

    // 9. The TUI should show the RESUMED header, confirming --resume/-c was passed
    await session.expectPassthroughHasText("RESUMED", 30_000);
  });

  test("resume TUI session with multiple repos reconnects with resume flag", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);

    // Multi-repo task envs use filepath.Dir(worktrees[0].WorktreePath) for the
    // workspace path so agentctl points at the task root and the per-repo
    // tracker fan-out works. This path is exercised by GetWorkspaceInfoForSession
    // → createExecutionFromSessionInfo → ResumePassthroughSession only when
    // len(worktrees) > 1, which the single-repo TUI test above never hits.

    // 1. Seed a second repo with one commit
    const gitEnv = {
      ...process.env,
      HOME: backend.tmpDir,
      GIT_AUTHOR_NAME: "E2E Test",
      GIT_AUTHOR_EMAIL: "e2e@test.local",
      GIT_COMMITTER_NAME: "E2E Test",
      GIT_COMMITTER_EMAIL: "e2e@test.local",
    };
    const repoDir = path.join(backend.tmpDir, "repos", "tui-multi-extra-repo");
    fs.mkdirSync(repoDir, { recursive: true });
    execSync("git init -b main", { cwd: repoDir, env: gitEnv });
    execSync('git commit --allow-empty -m "init"', { cwd: repoDir, env: gitEnv });
    const extraRepo = await apiClient.createRepository(seedData.workspaceId, repoDir, "main", {
      name: "tui-multi-extra-repo",
    });

    // 2. TUI profile + multi-repo task
    const tuiProfile = await createTUIProfile(apiClient, "TUI Multi-Repo Resume");
    await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "TUI Multi-Repo Resume Task",
      tuiProfile.id,
      {
        description: "hello from multi-repo resume",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId, extraRepo.id],
      },
    );

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    const card = kanban.taskCardByTitle("TUI Multi-Repo Resume Task");
    await expect(card).toBeVisible({ timeout: 15_000 });
    await card.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForPassthroughLoad();
    await session.waitForPassthroughLoaded();
    await session.expectPassthroughHasText("Mock Agent");

    await expect(session.stepperStep("Review")).toHaveAttribute("aria-current", "step", {
      timeout: 30_000,
    });

    // 3. Restart, reload, expect RESUMED — confirms multi-repo workspace path
    //    resolution preserves resume detection.
    await backend.restart();
    await testPage.reload();
    await session.waitForPassthroughLoad();
    await session.waitForPassthroughLoaded();
    await session.expectPassthroughHasText("RESUMED", 30_000);
  });
});

// ---------------------------------------------------------------------------
// Tests — multi-session resume
// ---------------------------------------------------------------------------

test.describe("Session resume (multi-session)", () => {
  test.describe.configure({ retries: 1 });

  test("both sessions reconnect after backend restart", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(180_000);

    // Direct regression for the production report: a task with 2 agent sessions
    // showing "Agent is not running" after a restart. Catches singleflight /
    // per-session executor-row reconciliation drift that wouldn't surface on a
    // single-session task.

    // 1. Create task; first session starts automatically
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Multi Session Resume Task",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    const session = await openTaskSession(testPage, "Multi Session Resume Task");
    await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 30_000,
    });
    await session.waitForChatIdle({ timeout: 15_000 });

    // 2. Open second session via the new-session dialog
    await session.openNewSessionDialog();
    await expect(session.newSessionDialog()).toBeVisible({ timeout: 5_000 });
    await session.newSessionPromptInput().fill("/e2e:simple-message");
    await session.newSessionStartButton().click();
    await expect(session.newSessionDialog()).not.toBeVisible({ timeout: 10_000 });

    // 3. Wait for both sessions to finish their first turn
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          if (sessions.length !== 2) return false;
          return sessions.every((s) => ["WAITING_FOR_INPUT", "COMPLETED"].includes(s.state));
        },
        { timeout: 60_000, message: "Waiting for both sessions to finish first turn" },
      )
      .toBe(true);

    // 4. Restart backend and reload
    await backend.restart();
    await testPage.reload();

    // 5. Both session tabs render in the UI. The original "Agent is not running"
    //    bug surfaced as broken sessions — frontend gates on session metadata
    //    that's only valid once reconcile completes for each session. If
    //    per-session reconcile drift left one stale, the corresponding tab
    //    would render in an error state or fail to mount.
    await expect(session.sessionTabByText("1")).toBeVisible({ timeout: 30_000 });
    await expect(session.sessionTabByText("2")).toBeVisible({ timeout: 15_000 });

    // 6. Backend state for BOTH sessions must reach an idle, error-free state.
    //    A single-session test would miss a singleflight bug that deduplicates
    //    the wrong session, since the duplicate-detection key collisions only
    //    surface with concurrent reconciliation.
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          if (sessions.length !== 2) return false;
          return sessions.every((s) => ["WAITING_FOR_INPUT", "COMPLETED"].includes(s.state));
        },
        {
          timeout: 60_000,
          message: "Waiting for both sessions to settle into idle state post-restart",
        },
      )
      .toBe(true);
  });

  test("keeps distinct model titles during multi-session resume", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }) => {
    test.setTimeout(180_000);

    const { agents } = await apiClient.listAgents();
    const mockAgent = agents.find((agent) => agent.name === "mock-agent") ?? agents[0];
    if (!mockAgent) throw new Error("mock-agent is not available for model resume test");
    const solProfile = await apiClient.createAgentProfile(mockAgent.id, "Resume Sol Profile", {
      model: "mock-smart",
    });
    const lunaProfile = await apiClient.createAgentProfile(mockAgent.id, "Resume Luna Profile", {
      model: "mock-fast",
    });

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Distinct Model Resume Task",
      solProfile.id,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    const firstSessionId = task.session_id;
    if (!firstSessionId) throw new Error("first model resume session was not created");

    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return sessions.some(
            (session) =>
              session.id === firstSessionId &&
              ["WAITING_FOR_INPUT", "COMPLETED"].includes(session.state),
          );
        },
        { timeout: 60_000, message: "first distinct-model session did not settle" },
      )
      .toBe(true);

    const second = await apiClient.launchSession(
      {
        task_id: task.id,
        agent_profile_id: lunaProfile.id,
        workflow_step_id: seedData.startStepId,
        prompt: "/e2e:simple-message",
      },
      60_000,
    );
    const secondSessionId = second.session_id;

    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return (
            sessions.length === 2 &&
            sessions.every((session) => ["WAITING_FOR_INPUT", "COMPLETED"].includes(session.state))
          );
        },
        { timeout: 60_000, message: "both distinct-model sessions did not settle" },
      )
      .toBe(true);

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.sessionTabBySessionId(firstSessionId)).toContainText("Mock Smart", {
      timeout: 30_000,
    });
    await expect(session.sessionTabBySessionId(secondSessionId)).toContainText("Mock Fast", {
      timeout: 15_000,
    });
    await prCapture.screenshot("desktop-distinct-model-tabs", {
      caption: "Distinct model labels remain visible across desktop session tabs",
    });

    await recordSessionTabHistory(testPage);
    await backend.restart();
    await testPage.reload();
    await session.waitForLoad();

    await expect(session.sessionTabBySessionId(firstSessionId)).toContainText("Mock Smart", {
      timeout: 60_000,
    });
    await expect(session.sessionTabBySessionId(secondSessionId)).toContainText("Mock Fast", {
      timeout: 30_000,
    });

    const history = await testPage.evaluate(
      () =>
        (window as Window & { __KANDEV_SESSION_TAB_HISTORY__?: SessionTabHistoryEntry[][] })
          .__KANDEV_SESSION_TAB_HISTORY__ ?? [],
    );
    const completeSamples = history.filter((sample) => {
      const first = sample.find((entry) => entry.id === firstSessionId);
      const secondEntry = sample.find((entry) => entry.id === secondSessionId);
      return !!first?.text && !!secondEntry?.text;
    });
    expect(completeSamples.length).toBeGreaterThan(0);
    for (const sample of completeSamples) {
      const first = sample.find((entry) => entry.id === firstSessionId);
      const secondEntry = sample.find((entry) => entry.id === secondSessionId);
      expect(first?.text).toContain("Mock Smart");
      expect(secondEntry?.text).toContain("Mock Fast");
    }
  });
});

// ---------------------------------------------------------------------------
// Tests — restart during active turn
// ---------------------------------------------------------------------------

test.describe("Session resume (active turn interrupted)", () => {
  test.describe.configure({ retries: 1 });

  test("restart mid-turn leaves session resumable and accepts new prompts", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(180_000);

    // Existing resume tests all restart while idle (Turn Finished /
    // WAITING_FOR_INPUT). This exercises the active-turn reconcile path:
    // the session is in RUNNING state with a non-terminal tool call when
    // the backend dies, so reconcile must demote to WAITING_FOR_INPUT and
    // CompletePendingToolCallsForTurn must close the dangling tool call.

    // 1. Create task with the auto-start agent (workflow handler launches it)
    await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mid-Turn Restart Task",
      seedData.agentProfileId,
      {
        // /slow takes ~15s, plenty of room to restart while RUNNING
        description: "/slow 15s",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    const session = await openTaskSession(testPage, "Mid-Turn Restart Task");

    // 2. Wait until the agent has clearly started working but is NOT idle yet —
    //    /slow emits its banner ~3s in, well before the 15s total.
    await expect(session.chat.getByText("Running slow response", { exact: false })).toBeVisible({
      timeout: 30_000,
    });
    await expect(session.idleInput()).not.toBeVisible({ timeout: 1_000 });

    // 3. Restart while the agent is still mid-turn
    await backend.restart();
    await testPage.reload();
    await session.waitForLoad();

    // 4. Existing partial output is still in the DB / visible
    await expect(session.chat.getByText("Running slow response", { exact: false })).toBeVisible({
      timeout: 15_000,
    });

    // 5. Reconcile must end up at idle so the user can interact again. This is
    //    the safety-net path: post-restart, no live agent process exists, so
    //    UpdateTaskSessionState moves RUNNING → WAITING_FOR_INPUT and
    //    CompletePendingToolCallsForTurn closes any dangling tool calls.
    await session.waitForChatIdle({ timeout: 90_000 });

    // 6. Send a fresh prompt — proves the resume path produced a usable
    //    execution (not stuck "Agent is not running") even though the previous
    //    turn was interrupted.
    await session.sendMessage("/e2e:simple-message");
    await session.expectChatResponseVisible("simple mock response", 0, { timeout: 30_000 });
  });
});
