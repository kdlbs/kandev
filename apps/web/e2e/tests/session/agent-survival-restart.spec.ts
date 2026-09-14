import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

// AC-EXECUTORS-SURVIVAL-001..003: with the capability enabled, a worktree
// executor's agent process is never stopped as part of a graceful backend
// restart (kill-paths #4/#5 are closed), so a turn already in flight when the
// backend goes down keeps running on its own and the re-tracked backend finds
// it exactly as it left it — no relaunch, no interrupted marker, and no
// premature move out of IN_PROGRESS on account of the restart itself
// (AC-EXECUTORS-SURVIVAL-003.5). The turn still completes normally once it
// finishes, and the workflow engine's own on_turn_complete transition then
// moves the task to REVIEW exactly as it would for an uninterrupted run
// (see session-resume.spec.ts for that baseline). `backend.restart()` sends
// SIGTERM to the backend's own process group only; agentctl already runs in
// its own group (see backend CLAUDE.md), so it is unaffected regardless of
// the capability — what the capability controls is whether the backend's own
// graceful-shutdown path stops it on the way down.
test.describe("Agent survival across backend restart", () => {
  test.describe.configure({ retries: 1 });

  test("a worktree session's in-flight turn survives a graceful backend restart", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);

    const releaseFeature = await backend.useEnv({
      KANDEV_FEATURES_AGENT_SURVIVAL: "true",
    });

    try {
      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Agent Survival Restart Task",
        seedData.agentProfileId,
        {
          description: "/slow 12",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
          executor_profile_id: seedData.worktreeExecutorProfileId,
        },
      );

      await testPage.goto(`/t/${task.id}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();

      // Confirm the agent actually launched, and that its turn is genuinely
      // in flight (past the initial "thinking" step), before pulling the
      // backend out from under it.
      const bootMessage = session.chat.getByText(/Started agent|Resumed agent/i);
      await expect(bootMessage).toBeVisible({ timeout: 30_000 });
      await expect(session.chat.getByText("Running slow response", { exact: false })).toBeVisible({
        timeout: 30_000,
      });
      await expect(apiClient.getTask(task.id)).resolves.toMatchObject({ state: "IN_PROGRESS" });

      // Restart the backend while the turn is still running.
      await backend.restart();

      // Reload so the page re-hydrates from the restarted backend's own SSR
      // boot payload, not just a live WS event the still-open tab happened to
      // receive.
      await testPage.reload();
      await session.waitForLoad();

      // The instance was re-tracked, not relaunched: exactly the one boot
      // message from the original launch, no second "Started agent" and no
      // "Resumed agent" from a resume flow this session never needed.
      await expect(session.chat.getByText(/Started agent|Resumed agent/i)).toHaveCount(1);

      // No manual-recovery banner: a re-tracked session is not an interrupted
      // one.
      await expect(session.recoveryFreshButton()).toHaveCount(0);
      await expect(session.recoveryResumeButton()).toHaveCount(0);

      // The turn that was still running across the restart must complete on
      // its own and report normally, rather than being abandoned into an idle
      // waiting-for-input state by startup reconciliation.
      await expect(session.chat.getByText("Slow response complete", { exact: false })).toBeVisible({
        timeout: 30_000,
      });
      await session.waitForChatIdle({ timeout: 15_000 });

      // The turn genuinely finished, so the workflow engine's own
      // on_turn_complete transition moves the task to REVIEW exactly as it
      // would without any restart (session-resume.spec.ts exercises the same
      // seedData workflow's baseline). What AC-EXECUTORS-SURVIVAL-003.5 rules
      // out is a move to REVIEW *on account of the restart itself* — the task
      // must never gain the interrupted marker.
      await expect
        .poll(async () => (await apiClient.getTask(task.id)).state, {
          timeout: 30_000,
          message: "Task did not advance to REVIEW after the turn completed",
        })
        .toBe("REVIEW");
      const reconciledTask = await apiClient.getTask(task.id);
      expect(reconciledTask.metadata?.interrupted_at).toBeUndefined();

      // The agent still works after the restart: a fresh prompt gets a
      // response through the re-tracked, still-live instance.
      await session.sendMessage("/e2e:simple-message");
      await session.expectChatResponseVisible("simple mock response", 0, { timeout: 30_000 });

      // Board view: the card must not show the interrupted affordance either.
      const kanban = new KanbanPage(testPage);
      await kanban.goto();
      const card = kanban.taskCard(task.id);
      await expect(card).toBeVisible({ timeout: 20_000 });
      await expect(card.getByTestId("task-state-interrupted")).toHaveCount(0);
    } finally {
      await releaseFeature();
    }
  });
});
