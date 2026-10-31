/**
 * Phone coverage for OpenCode background inspection (REQ-AGENTS-BACKGROUND-WORK-009).
 * Same deterministic OpenCode 2.2.0 launch fixture as the desktop suite, driven
 * through the shared chip drawer and the focused full-height detail surface.
 *
 * Observation-only scope: the producer ends delivery with the foreground
 * prompt, so the summary shows unknown activity and no unsupported controls.
 */
import { expect, test } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

const OPENCODE_FIXTURE_SCRIPT = "/opencode-background-fixture";
const SHELL_IDS = ["ses_e2e_shell_ci", "ses_e2e_shell_await", "ses_e2e_shell_tests"];
const UNSENT_DRAFT = "keep this draft while I inspect the jobs";

async function createOpenCodeProfile(apiClient: ApiClient): Promise<string> {
  let agentId = "";
  await expect
    .poll(
      async () => {
        const { agents } = await apiClient.listAgents();
        agentId = agents.find((agent) => agent.name === "opencode-acp")?.id ?? "";
        return agentId;
      },
      { message: "opencode-acp agent should be registered", timeout: 30_000 },
    )
    .not.toBe("");

  const profile = await apiClient.createAgentProfile(agentId, `OpenCode Mobile E2E ${Date.now()}`, {
    model: "mock-fast",
  });
  return profile.id;
}

test.describe("OpenCode background inspection on a phone", () => {
  test.beforeAll(async ({ backend }) => {
    await backend.restart({ KANDEV_MOCK_PROVIDERS: "opencode-acp" });
  });

  test.afterAll(async ({ backend }) => {
    await backend.restart();
  });

  test("drawer summary and full-height detail preserve the draft and unsupported controls stay absent", async ({
    apiClient,
    seedData,
    testPage,
  }) => {
    test.setTimeout(120_000);
    const agentProfileId = await createOpenCodeProfile(apiClient);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "OpenCode phone background",
      agentProfileId,
      {
        description: OPENCODE_FIXTURE_SCRIPT,
        repository_ids: [seedData.repositoryId],
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return sessions[0]?.state ?? "";
        },
        { message: "the foreground turn should settle", timeout: 30_000 },
      )
      .toBe("WAITING_FOR_INPUT");

    const editor = await session.fillMessage(UNSENT_DRAFT);

    const chip = testPage.getByTestId("background-work-chip");
    await expect(chip).toBeVisible({ timeout: 30_000 });
    await chip.tap();

    const drawer = testPage.getByTestId("background-work-drawer-content");
    await expect(drawer).toBeVisible();
    const summary = drawer.getByTestId("background-work-summary-list");
    await expect(summary).toBeVisible();
    for (const shellID of SHELL_IDS) {
      await expect(summary.getByTestId(`background-work-summary-item-${shellID}`)).toBeVisible();
    }
    await expect(summary).toContainText("3 unknown");

    // Resolve one job as failed with a real exit code and retained output. The
    // legacy OpenCode producer never delivers this, and the phone detail must
    // show the same retained outcome as desktop.
    let sessionId = task.session_id ?? "";
    if (!sessionId) {
      await expect
        .poll(
          async () => {
            const { sessions } = await apiClient.listTaskSessions(task.id);
            sessionId = sessions[0]?.id ?? "";
            return sessionId;
          },
          { message: "session row should be created", timeout: 30_000 },
        )
        .not.toBe("");
    }
    await apiClient.recordBackgroundWorkload(sessionId, {
      work_id: SHELL_IDS[2],
      run_id: SHELL_IDS[2],
      kind: "shell",
      title: "Local tests",
      state: "failed",
      exit_code: 2,
      output: "checks failed: 2 tests",
      revision: 99,
    });
    await expect(summary.getByTestId(`background-work-state-${SHELL_IDS[2]}`)).toHaveText("Failed");

    // A job opens a focused full-height surface with one scroll owner.
    await summary.getByTestId(`background-work-open-button-${SHELL_IDS[2]}`).tap();
    const detail = testPage.getByTestId("background-work-drawer-detail");
    await expect(detail).toBeVisible();
    const viewport = testPage.viewportSize();
    const detailBox = await detail.boundingBox();
    expect(detailBox).not.toBeNull();
    expect(detailBox!.height).toBeGreaterThanOrEqual((viewport?.height ?? 0) * 0.85);

    // Coarse-pointer actions meet the 44px minimum.
    const back = testPage.getByTestId("background-work-drawer-back");
    const backBox = await back.boundingBox();
    expect(backBox).not.toBeNull();
    expect(backBox!.height).toBeGreaterThanOrEqual(44);
    expect(backBox!.width).toBeGreaterThanOrEqual(44);

    // No document-level horizontal overflow while the detail surface is open.
    const horizontalOverflow = await testPage.evaluate(
      () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
    );
    expect(horizontalOverflow).toBeLessThanOrEqual(0);

    // Retained outcome travels to the phone: exit status and output.
    await expect(testPage.getByTestId("background-work-drawer-exit")).toHaveText("Exit 2");
    await expect(detail).toContainText("checks failed: 2 tests");

    // Unsupported controls stay absent after the terminal transition.
    await expect(testPage.getByTestId("stop-workload-button")).toHaveCount(0);
    await expect(testPage.getByTestId("interrupt-workload-button")).toHaveCount(0);

    // Back restores the list context.
    await back.tap();
    await expect(summary).toBeVisible();
    await expect(summary.getByTestId(`background-work-summary-item-${SHELL_IDS[2]}`)).toBeVisible();

    // Dismissal restores focus to the chip and does not disturb the draft.
    await drawer.getByRole("button", { name: "Close background work" }).tap();
    await expect(drawer).toBeHidden();
    await expect(editor).toHaveText(UNSENT_DRAFT);
    await expect(chip).toBeFocused();
  });
});
