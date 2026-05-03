import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

/**
 * Regression: a CLI passthrough agent that fast-fails on resume (e.g. real
 * Claude CLI exits "No conversation found to continue") should auto-recover
 * by relaunching once with a fresh command (no resume flag) instead of
 * leaving the user stuck with a red banner.
 *
 * Driven via the mock-agent's --fail-on-resume flag: when invoked with
 * -c / --resume the mock prints the canonical error and exits 1; otherwise
 * it boots normally. The lifecycle manager's resume-fallback path should
 * detect the fast-fail and relaunch fresh.
 */

async function createTUIProfileWithFailOnResume(apiClient: ApiClient, name: string) {
  const { agents } = await apiClient.listAgents();
  return apiClient.createAgentProfile(agents[0].id, name, {
    model: "mock-fast",
    cli_passthrough: true,
    cli_flags: [{ description: "fail on resume", flag: "--fail-on-resume", enabled: true }],
  });
}

test.describe("Session resume — CLI fallback after fast-fail", () => {
  test.describe.configure({ retries: 1 });

  test("relaunches without resume flag when CLI exits fast on -c", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);

    const profile = await createTUIProfileWithFailOnResume(apiClient, "TUI Resume Fallback");

    await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "TUI Resume Fallback Task",
      profile.id,
      {
        description: "hello from resume fallback test",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const card = kanban.taskCardByTitle("TUI Resume Fallback Task");
    await expect(card).toBeVisible({ timeout: 15_000 });
    await card.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForPassthroughLoad();
    await session.waitForPassthroughLoaded();

    // Initial fresh launch: --fail-on-resume is present but no resume flag,
    // so the mock-agent boots normally and renders the standard header.
    await session.expectPassthroughHasText("Mock Agent");

    // Wait for the workflow step to advance so the session is in
    // WAITING_FOR_INPUT — the precondition for resume on next launch.
    await expect(session.stepperStep("Review")).toHaveAttribute("aria-current", "step", {
      timeout: 30_000,
    });

    // Restart the backend, then reload the page to trigger a session resume.
    // The first launch attempt will pass -c → mock prints "No conversation
    // found to continue" and exits 1 → fast-fail detector triggers fallback.
    await backend.restart();
    await testPage.reload();

    await session.waitForPassthroughLoad();
    await session.waitForPassthroughLoaded();

    // The fallback writes a yellow banner to the terminal before relaunching.
    await session.expectPassthroughHasText("starting a fresh session", 30_000);

    // After the fallback launch, the mock-agent boots without the resumed
    // header — confirms the second launch dropped the resume flag.
    await session.expectPassthroughHasText("Mock Agent", 30_000);
    await session.expectPassthroughNotHasText("RESUMED");
  });
});
