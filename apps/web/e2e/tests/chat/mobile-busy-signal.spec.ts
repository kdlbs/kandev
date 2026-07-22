import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { waitForActiveSessionForegroundActivity } from "../../helpers/session-store";

// Mobile (Pixel 5) coverage for ADR-0049. Filename matches
// /mobile-.*\.spec\.ts/ so the `mobile-chrome` project picks it up. The composer
// gating and the working affordance derive from the same shared hooks the
// desktop path uses, so this asserts the operator-visible outcome holds at
// mobile width: a background-idle session shows "working" while the composer
// accepts input, then flips to done once the background task finishes.

test.describe("Mobile fine-grained busy signal", () => {
  test.describe.configure({ retries: 1 });

  test("async subagent allows instant mobile send and clears on singleton ID-less completion", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(150_000);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile async subagent lifecycle",
      seedData.agentProfileId,
      {
        description: "/async-subagent-lifecycle 25s",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(testPage.getByText("Foreground response after async launch.")).toBeVisible({
      timeout: 25_000,
    });
    await expect(session.idleInput()).toBeVisible({ timeout: 25_000 });
    await expect(session.agentStatus()).toBeVisible();
    await waitForActiveSessionForegroundActivity(testPage, "background");

    // Use the shipped Pixel 5 composer button path; the prompt must be posted,
    // not queued, and foreground must take visual precedence over the child.
    await session.sendMessageViaButton("/slow 3s");
    await expect(testPage.getByText("/slow 3s")).toBeVisible({ timeout: 15_000 });
    await expect(testPage.getByTestId("queue-chip")).not.toBeVisible();
    await waitForActiveSessionForegroundActivity(testPage, "generating");

    await expect(session.idleInput()).toBeVisible({ timeout: 20_000 });
    await waitForActiveSessionForegroundActivity(testPage, "background");
    await expect(session.agentStatus()).not.toBeVisible({ timeout: 45_000 });
    await waitForActiveSessionForegroundActivity(testPage, null);
    await expect(session.agentStatus()).not.toBeVisible();
  });

  test("execution teardown clears a missing async completion on mobile", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(100_000);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile async subagent teardown",
      seedData.agentProfileId,
      {
        description: "/async-subagent-teardown",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("auto-started task did not return a session id");

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.idleInput()).toBeVisible({ timeout: 25_000 });
    await expect(session.agentStatus()).toBeVisible();
    await waitForActiveSessionForegroundActivity(testPage, "background");

    await apiClient.stopSession({
      session_id: task.session_id,
      reason: "e2e teardown",
      force: true,
    });
    await expect(session.agentStatus()).not.toBeVisible({ timeout: 20_000 });
    await waitForActiveSessionForegroundActivity(testPage, null);
    await expect(session.agentStatus()).not.toBeVisible();
  });

  test("background-idle session shows working AND accepts input at mobile width", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    // Drive the background window via the auto-started first turn (the task
    // description) rather than a sendMessage follow-up: the live turn reliably
    // reaches the mobile client while it is still running (same approach as
    // mobile-empty-turn.spec.ts).
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile busy signal",
      seedData.agentProfileId,
      {
        description: "/detached-background 30s",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // The auto-started turn is working…
    await expect(session.agentStatus()).toBeVisible({ timeout: 20_000 });

    // …and once it yields to background work the composer flips to accept-input
    // (idle placeholder) while the status still reads "working" — both facts
    // visible at once at mobile width.
    await expect(session.idleInput()).toBeVisible({ timeout: 25_000 });
    await expect(session.agentStatus()).toBeVisible();
    await waitForActiveSessionForegroundActivity(testPage, "background");

    // A mobile button submission during the background-only window must start
    // a new foreground turn immediately, never divert the prompt to the queue.
    await session.sendMessageViaButton("/slow 5s");
    await expect(testPage.getByText("/slow 5s")).toBeVisible({ timeout: 15_000 });
    await expect(testPage.getByTestId("queue-chip")).not.toBeVisible();
    await expect(session.idleInput()).not.toBeVisible({ timeout: 5_000 });
    await waitForActiveSessionForegroundActivity(testPage, "generating");

    // Foreground completion exposes the older detached work again until its
    // own lifecycle completion arrives.
    await expect(session.idleInput()).toBeVisible({ timeout: 20_000 });
    await expect(session.agentStatus()).toBeVisible();
    await waitForActiveSessionForegroundActivity(testPage, "background");

    // After the detached task completes, the working affordance clears.
    await expect(session.agentStatus()).not.toBeVisible({ timeout: 40_000 });
    await expect(session.idleInput()).toBeVisible({ timeout: 10_000 });
  });

  test("background-idle substate survives a reload at mobile width", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    // Long background window so it is still held after the reload lands.
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile busy signal reload",
      seedData.agentProfileId,
      {
        description: "/detached-background 20s",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // Reach the background-idle window.
    await expect(session.agentStatus()).toBeVisible({ timeout: 20_000 });
    await expect(session.idleInput()).toBeVisible({ timeout: 25_000 });
    await expect(session.agentStatus()).toBeVisible();

    // Reload mid-window: a fresh mobile client. The accept-input + working
    // affordance must come straight from the boot payload (no persisted value,
    // no activity_changed WS flip due) — ADR-0049.
    await testPage.reload();
    await session.waitForLoad();

    await expect(session.idleInput()).toBeVisible({ timeout: 15_000 });
    await expect(session.agentStatus()).toBeVisible();
  });
});
