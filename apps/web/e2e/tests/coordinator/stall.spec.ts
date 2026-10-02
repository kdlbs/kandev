// AC-COORDINATOR-NEEDS-YOU-005: a real (not seeded) `task.stalled` sweep
// result must reach the Needs you screen live, as a Decide-now stall item
// with working evidence (docs/specs/coordinator/requirements/needs-you.md,
// #stall-records). The session is seeded execution-less via the E2E test
// harness (no mock agent process is launched) so the backend's one-minute
// stall sweep is the only thing that can produce the event this test waits on.
import { test, expect } from "../../fixtures/test-base";
import { watchWs } from "../../helpers/causal-waits";
import { linkToCoordinatorNeedsYou } from "../../../lib/coordinator/links";

test.describe("Coordinator stall detection", () => {
  test("a real task.stalled sweep renders a Decide-now stall item with evidence, live (AC .005)", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(220_000);

    // The sweep (active_session_stall.go) also heals a session back to
    // WAITING_FOR_INPUT once it has been silent past twice the threshold, in
    // the same tick that first detects the stall if both conditions are
    // already true. A threshold below the sweep's 1-minute period lets a
    // single tick jump straight past "detected" to "healed", so the frontend
    // never observes the card. 75s (versus the 60s tick) guarantees at least
    // one tick lands strictly between the threshold and its 2x heal grace:
    // restarting the backend resets the sweep's tick phase to ~0, so the
    // first tick after this task is created (~60s later) is still below
    // threshold, and the second (~120s) is comfortably inside the window.
    const release = await backend.useEnv({ KANDEV_TASK_STALL_DETECTION_THRESHOLD: "75s" });
    try {
      const coordinator = await apiClient.createCoordinator(seedData.workspaceId, {
        name: "Stall Coordinator",
        agent_profile_id: seedData.agentProfileId,
        executor_profile_id: seedData.worktreeExecutorProfileId,
        task_agent_profile_id: seedData.agentProfileId,
        task_executor_profile_id: seedData.worktreeExecutorProfileId,
      });

      const title = "Stalled Task Fixture";
      const task = await apiClient.createTask(seedData.workspaceId, title, {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      });
      // No agent process is ever launched for this session: the sweep's
      // in-memory execution registry has nothing live for it from the start.
      await apiClient.seedTaskSession(task.id, { state: "RUNNING" });

      const ws = watchWs(testPage);
      await testPage.goto(linkToCoordinatorNeedsYou(seedData.workspaceId, coordinator.id));

      const stallEvent = ws.waitForEvent("coordinator.updated", {
        where: (payload) =>
          payload.workspace_id === seedData.workspaceId &&
          payload.coordinator_id === coordinator.id,
        // Expected detection lands on the sweep's second tick (~120s post
        // task-creation, see the threshold comment above); this leaves
        // margin without reaching the ~150s heal grace that would clear it.
        timeout: 180_000,
      });
      await stallEvent;

      const card = testPage.getByTestId(`needs-you-item-${task.id}`);
      await expect(card).toBeVisible({ timeout: 15_000 });
      await expect(card).toContainText(title);
      await expect(card.getByText("Decide now")).toBeVisible();
      await expect(card.getByText("No activity for", { exact: false })).toBeVisible();
      await expect(card.getByText("Resuming or restarting the task")).toBeVisible();

      await expect(card.getByRole("link", { name: "Open task" })).toBeVisible();
      const showEvidence = card.getByRole("button", { name: "Show the evidence" });
      await expect(showEvidence).toBeVisible();
      await showEvidence.click();

      // Radix Popover content renders in a portal outside the card, so the
      // assertion scopes to the page rather than `card`.
      await expect(testPage.getByText("Stalled for")).toBeVisible();
      await expect(testPage.getByText("Last event at")).toBeVisible();
      await expect(testPage.getByText("Detected at")).toBeVisible();
      // The evidence's agent status reads task.statusSummary.primary_session.state
      // (StallEvidenceContent -> isRunningSessionState), a live projection built
      // from session events. The harness's seedTaskSession call publishes a plain
      // state-changed event with no is_primary/primary_session_id marker, so this
      // session is never adopted as the task's primary session in that projection
      // -- correctly, since no agent process was ever launched for it.
      await expect(testPage.getByText("Agent not running")).toBeVisible();
    } finally {
      await release();
    }
  });
});
