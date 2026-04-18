import fs from "node:fs";
import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

/**
 * Verifies the UX for the inline prepare-progress card that renders below the
 * initial user prompt:
 *   - While running: card is expanded and streams the setup script output.
 *   - On clean success: card auto-collapses (still visible as a summary row).
 *   - On step failure: card stays expanded so the error is immediately visible;
 *     user can click to collapse.
 *   - Without a setup script: card still renders, auto-collapses on success.
 *
 * Each test creates a fresh executor profile carrying the desired prepare
 * script — this hits the same `runSetupScript` path users exercise via the
 * repository setup_script placeholder in profile templates, without mutating
 * the worker-scoped seed profile (avoids cross-test contamination).
 */
test.describe("Setup script progress UX", () => {
  async function createWorktreeProfileWithScript(
    apiClient: import("../../helpers/api-client").ApiClient,
    script: string,
    name: string,
  ): Promise<{ id: string }> {
    const { executors } = await apiClient.listExecutors();
    const worktreeExec = executors.find((e) => e.type === "worktree");
    if (!worktreeExec) throw new Error("No worktree executor found");
    return apiClient.createExecutorProfile(worktreeExec.id, name, { prepare_script: script });
  }

  test("streams setup script output while running, collapses after success", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);

    // Slow `git fetch` (worktree sync step) to keep the prepare panel reliably
    // expanded while the page mounts and the WS subscribes — without this the
    // whole prepare can finish before the client is listening, so events are
    // missed and the running state is never observed. Same shim pattern as
    // `long-prepare-panels.spec.ts`. The final `prepare.completed` payload
    // carries all captured stdout, so the streaming assertion is still valid
    // even if the page races past the per-progress events.
    const delayFile = path.join(backend.tmpDir, "git-delay-ms");
    let profile: { id: string } | null = null;
    try {
      // Inside the try so the finally always cleans up — if profile creation
      // throws after the shim is written, leaving the delay file behind would
      // contaminate every other test in this worker.
      fs.writeFileSync(delayFile, "5000");

      const setupScript = [
        "echo '[setup] installing deps'",
        "sleep 1",
        "echo '[setup] ready'",
      ].join("; ");
      profile = await createWorktreeProfileWithScript(
        apiClient,
        setupScript,
        `E2E Streaming ${Date.now()}`,
      );

      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Setup Script Streaming",
        seedData.agentProfileId,
        {
          description: "/e2e:simple-message",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
          executor_profile_id: profile.id,
        },
      );

      await testPage.goto(`/t/${task.id}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();

      // Panel appears inline in the chat, expanded while running.
      const panel = testPage.getByTestId("prepare-progress-panel");
      await expect(panel).toBeVisible({ timeout: 15_000 });
      await expect(panel).toHaveAttribute("data-status", "preparing");
      await expect(panel).toHaveAttribute("data-expanded", "true");
      await expect(panel.getByTestId("prepare-progress-header-spinner")).toBeVisible();

      // Setup script output reaches the expanded step list — either streamed
      // in real time or captured from the final `prepare.completed` payload.
      await expect(panel).toContainText("[setup] installing deps", { timeout: 30_000 });

      // On clean success the panel stays visible but auto-collapses.
      await expect(panel).toHaveAttribute("data-status", "completed", { timeout: 30_000 });
      await expect(panel).toHaveAttribute("data-expanded", "false");
    } finally {
      if (fs.existsSync(delayFile)) fs.unlinkSync(delayFile);
      if (profile) {
        await apiClient.deleteExecutorProfile(profile.id).catch(() => {
          // Profile may already be deleted if the test tore down mid-run.
        });
      }
    }
  });

  test("stays expanded on setup script failure with output visible", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);

    // Exit non-zero with output on both streams — the panel must surface
    // both so the user can diagnose without diving into backend logs. The
    // upfront sleep gives the page + WS time to subscribe before the script
    // terminates; without it the failure fires faster than the frontend can
    // connect and we miss every event.
    const failingScript = [
      "echo '[setup] starting install'",
      "sleep 4",
      "echo 'ENOENT: package-lock.json missing' 1>&2",
      "exit 1",
    ].join("; ");
    const profile = await createWorktreeProfileWithScript(
      apiClient,
      failingScript,
      `E2E Failing ${Date.now()}`,
    );

    try {
      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Setup Script Failure",
        seedData.agentProfileId,
        {
          description: "/e2e:simple-message",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
          executor_profile_id: profile.id,
        },
      );

      await testPage.goto(`/t/${task.id}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();

      const panel = testPage.getByTestId("prepare-progress-panel");

      // Setup script failures are non-fatal (agent still starts), so the panel
      // transitions preparing → completed_with_error rather than the fatal
      // `failed` state — and stays expanded so the error is readable.
      await expect(panel).toBeVisible({ timeout: 15_000 });
      await expect(panel).toHaveAttribute("data-status", "completed_with_error", {
        timeout: 30_000,
      });
      await expect(panel).toHaveAttribute("data-expanded", "true");

      // Both stdout and stderr from the failing script must remain visible so
      // the user can diagnose — this is the core fix of this change.
      await expect(panel).toContainText("[setup] starting install");
      await expect(panel).toContainText("ENOENT: package-lock.json missing");

      // User can click the card header to collapse it.
      await panel.getByTestId("prepare-progress-toggle").click();
      await expect(panel).toHaveAttribute("data-expanded", "false");
    } finally {
      await apiClient.deleteExecutorProfile(profile.id).catch(() => {});
    }
  });

  test("prepare panel persists after page refresh", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);

    const delayFile = path.join(backend.tmpDir, "git-delay-ms");
    const setupScript = "echo '[setup] persistence check'";
    const profile = await createWorktreeProfileWithScript(
      apiClient,
      setupScript,
      `E2E Persist ${Date.now()}`,
    );

    try {
      fs.writeFileSync(delayFile, "5000");

      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Prepare Panel Persist",
        seedData.agentProfileId,
        {
          description: "/e2e:simple-message",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
          executor_profile_id: profile.id,
        },
      );

      await testPage.goto(`/t/${task.id}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();

      const panel = testPage.getByTestId("prepare-progress-panel");

      // Wait for preparation to complete.
      await expect(panel).toBeVisible({ timeout: 15_000 });
      await expect(panel).toHaveAttribute("data-status", "completed", { timeout: 30_000 });

      // Expand the panel (auto-collapsed on success) and verify step data.
      await panel.getByTestId("prepare-progress-toggle").click();
      await expect(panel).toContainText("Validate repository", { timeout: 5_000 });
      await expect(panel).toContainText("Run setup script", { timeout: 5_000 });

      // Reload the page — the panel AND step data must survive the round-trip through SSR.
      await testPage.reload();
      await session.waitForLoad();

      const panelAfterReload = testPage.getByTestId("prepare-progress-panel");
      await expect(panelAfterReload).toBeVisible({ timeout: 10_000 });
      // Status should still be "completed" (not "preparing").
      await expect(panelAfterReload).toHaveAttribute("data-status", "completed");

      // Step details must persist — not just the panel header.
      // Expand the panel to see steps (auto-collapsed on success).
      await panelAfterReload.getByTestId("prepare-progress-toggle").click();
      await expect(panelAfterReload).toContainText("Validate repository");
      await expect(panelAfterReload).toContainText("Run setup script");
    } finally {
      if (fs.existsSync(delayFile)) fs.unlinkSync(delayFile);
      await apiClient.deleteExecutorProfile(profile.id).catch(() => {});
    }
  });

  test("renders collapsed on success when no setup script is configured", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(60_000);

    // Slow the git fetch so prepare takes long enough for the page+WS to
    // connect before the completed event fires — without this the events
    // broadcast before the client is subscribed, and the panel never shows.
    const delayFile = path.join(backend.tmpDir, "git-delay-ms");
    try {
      fs.writeFileSync(delayFile, "5000");

      // Default seeded worktree profile — empty prepare_script. The worktree
      // validate/sync/create steps still run (card appears), and with no
      // error/warning the card auto-collapses once done.
      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "No Setup Script",
        seedData.agentProfileId,
        {
          description: "/e2e:simple-message",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
          executor_profile_id: seedData.worktreeExecutorProfileId,
        },
      );

      await testPage.goto(`/t/${task.id}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();

      // Panel stays visible but auto-collapses on a clean run. This replaces
      // the pre-redesign "disappears entirely" behavior — users want to be
      // able to expand it and read what ran even on a happy path.
      const panel = testPage.getByTestId("prepare-progress-panel");
      await expect(panel).toBeVisible({ timeout: 30_000 });
      await expect(panel).toHaveAttribute("data-status", "completed", { timeout: 30_000 });
      await expect(panel).toHaveAttribute("data-expanded", "false");
    } finally {
      if (fs.existsSync(delayFile)) fs.unlinkSync(delayFile);
    }
  });
});
