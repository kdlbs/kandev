/**
 * Phone coverage for OpenCode background inspection (REQ-AGENTS-BACKGROUND-WORK-009).
 * Same deterministic OpenCode 2.2.0 launch fixture as the desktop suite. Legacy
 * delivery cannot establish liveness, so the running-only composer chip stays
 * absent and the independent Panels entry opens History.
 *
 * Observation-only scope: the producer ends delivery with the foreground
 * prompt, so History keeps unknown activity and no unsupported controls appear.
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

  test("independent Panels entry keeps unknown history and the full-height detail honest", async ({
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

    // No verified running job means no composer chip.
    await expect(testPage.getByTestId("background-work-chip")).toHaveCount(0);
    await expect(testPage.getByTestId("background-work-history-chip")).toHaveCount(0);

    await testPage.getByTestId("mobile-session-nav-panels").tap();
    await testPage.getByTestId("mobile-background-work-open").tap();

    const panel = testPage.getByTestId("background-work-mobile-panel");
    await expect(panel).toBeVisible();
    await expect(testPage.getByTestId("background-work-tab-history")).toHaveAttribute(
      "data-state",
      "active",
    );
    for (const shellID of SHELL_IDS) {
      await expect(testPage.getByTestId(`background-workload-card-${shellID}`)).toBeVisible();
      await expect(testPage.getByTestId(`background-work-state-${shellID}`)).toHaveText("Unknown");
    }

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
    await expect(testPage.getByTestId(`background-work-state-${SHELL_IDS[2]}`)).toHaveText(
      "Failed",
    );

    // A job opens a focused full-height surface with one scroll owner.
    await testPage.getByTestId(`open-workload-${SHELL_IDS[2]}`).tap();
    const detail = testPage.getByTestId("background-work-drawer-detail");
    await expect(detail).toBeVisible();
    const viewport = testPage.viewportSize();
    const detailBox = await detail.boundingBox();
    expect(detailBox).not.toBeNull();
    expect(detailBox!.height).toBeGreaterThanOrEqual((viewport?.height ?? 0) * 0.8);

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

    // Back restores the list context, including its History tab.
    await back.tap();
    await expect(testPage.getByTestId("background-work-tab-history")).toHaveAttribute(
      "data-state",
      "active",
    );
    await expect(testPage.getByTestId(`background-workload-card-${SHELL_IDS[2]}`)).toBeVisible();

    // Dismissal returns focus to the Panels trigger and preserves the draft.
    await testPage.getByTestId("background-work-overview-close").tap();
    await expect(panel).toHaveCount(0);
    await expect(editor).toHaveText(UNSENT_DRAFT);
    await expect(testPage.getByTestId("mobile-session-nav-panels")).toBeFocused();
  });
});
