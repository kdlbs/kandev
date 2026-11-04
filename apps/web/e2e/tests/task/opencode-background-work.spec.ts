/**
 * E2E coverage for OpenCode background inspection (REQ-AGENTS-BACKGROUND-WORK-007
 * and -009). The mock provider is registered under the real `opencode-acp` agent
 * identity through KANDEV_MOCK_PROVIDERS, so the ACP adapter, normalizer, task
 * projection, WebSocket path, and browser renderer are all production code. The
 * producer frames are deterministic and use the captured OpenCode 2.2.0 launch
 * shape (background=true input, metadata.shellID plus metadata.status=running).
 *
 * This is observation-only evidence. OpenCode 2.2.0 ends its ACP subscription
 * with the foreground prompt, so the suite asserts the honest unknown state and
 * the absence of Stop, Interrupt, and input controls. It does not claim
 * automatic follow-up delivery, which requires the upstream producer from the
 * plan's Task 02.
 */
import { expect, test } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { firstSessionId } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";

const OPENCODE_FIXTURE_SCRIPT = "/opencode-background-fixture";
const SHELL_IDS = ["ses_e2e_shell_ci", "ses_e2e_shell_await", "ses_e2e_shell_tests"];

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

  const profile = await apiClient.createAgentProfile(agentId, `OpenCode E2E ${Date.now()}`, {
    model: "mock-fast",
  });
  return profile.id;
}

test.describe("OpenCode background inspection", () => {
  test.beforeAll(async ({ backend }) => {
    await backend.restart({ KANDEV_MOCK_PROVIDERS: "opencode-acp" });
  });

  test.afterAll(async ({ backend }) => {
    await backend.restart();
  });

  test("three identified launches stay inspectable after the foreground settles", async ({
    apiClient,
    seedData,
    testPage,
  }) => {
    test.setTimeout(120_000);
    const agentProfileId = await createOpenCodeProfile(apiClient);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "OpenCode background inspection",
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
    const settledSessionId = await firstSessionId(apiClient, task.id);
    await expect
      .poll(
        async () => {
          const { workloads } = await apiClient.listBackgroundWorkloads(settledSessionId);
          return {
            count: workloads.length,
            running: workloads.filter((workload) => workload.state === "running").length,
          };
        },
        { message: "the three legacy launches should settle to unknown", timeout: 30_000 },
      )
      .toEqual({ count: SHELL_IDS.length, running: 0 });

    // AC-007.1: each confirmed launch keeps one shared workload identity, even
    // after the launch tool finished and the foreground prompt settled. Legacy
    // delivery cannot establish liveness, so no composer chip is claimed; the
    // independent entry keeps all three unknown records inspectable.
    await testPage.getByTestId("dockview-add-panel-btn").first().click();
    await testPage.getByTestId("add-panel-background-work").click();
    const overview = testPage.getByTestId("background-work-overview");
    await expect(overview).toBeVisible();
    await expect(testPage.getByTestId("background-work-tab-history")).toHaveAttribute(
      "data-state",
      "active",
    );
    for (const shellID of SHELL_IDS) {
      await expect(testPage.getByTestId(`background-workload-card-${shellID}`)).toBeVisible();
    }
    await expect(testPage.getByTestId("background-work-chip")).toHaveCount(0);

    // AC-007.4 / AC-008.5: legacy delivery cannot establish liveness, so
    // History keeps the honest unknown status instead of claiming completion.
    for (const shellID of SHELL_IDS) {
      await expect(testPage.getByTestId(`background-work-state-${shellID}`)).toHaveText("Unknown");
    }

    // AC-009.4: a job opens its own central detail tab, with unavailable output
    // and no controls the runtime never advertised.
    await testPage.getByTestId(`open-workload-${SHELL_IDS[2]}`).click();
    const detail = testPage.getByTestId(`background-work-detail-${SHELL_IDS[2]}`);
    await expect(detail).toBeVisible();
    await expect(detail).toContainText("Output is not available for this job");
    await expect(testPage.getByTestId("stop-workload-button")).toHaveCount(0);
    await expect(testPage.getByTestId("interrupt-workload-button")).toHaveCount(0);
    await expect(testPage.getByTestId("background-work-actions-unavailable")).toBeVisible();

    // AC-009.2: the idle foreground accepts an ordinary new prompt while the
    // earlier jobs remain inspectable (AC-007.5). Return to Chat first: opening
    // a job focused its own central tab.
    await session.clickSessionChatTab();
    await session.waitForDirectInput(30_000);
    await session.sendMessage("Report the job status when you have it.");
    await expect(session.activeChat()).toContainText("Report the job status when you have it.");
    await expect(testPage.getByTestId("background-work-chip")).toHaveCount(0);
  });

  test("history remains reachable after a terminal job and no active count", async ({
    apiClient,
    seedData,
    testPage,
  }) => {
    test.setTimeout(120_000);
    const agentProfileId = await createOpenCodeProfile(apiClient);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "OpenCode background history",
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
    await expect
      .poll(
        async () => {
          const { workloads } = await apiClient.listBackgroundWorkloads(sessionId);
          return workloads.length;
        },
        { message: "the fixture should register three workloads", timeout: 30_000 },
      )
      .toBe(SHELL_IDS.length);

    // Resolve every job through the shared API with a terminal state. The
    // viewer, not a provider branch, must keep history reachable once the
    // running count is zero (AC-009.5).
    for (const shellID of SHELL_IDS) {
      await apiClient.recordBackgroundWorkload(sessionId, {
        work_id: shellID,
        run_id: shellID,
        kind: "shell",
        title: shellID,
        state: "completed",
        exit_code: 0,
        revision: 99,
      });
    }

    await expect(testPage.getByTestId("background-work-chip")).toHaveCount(0);
    await expect(testPage.getByTestId("background-work-history-chip")).toHaveCount(0);

    await testPage.getByTestId("dockview-add-panel-btn").first().click();
    await testPage.getByTestId("add-panel-background-work").click();
    await expect(testPage.getByTestId("background-work-overview")).toBeVisible();
    await expect(testPage.getByTestId("background-work-tab-history")).toHaveAttribute(
      "data-state",
      "active",
    );
    for (const shellID of SHELL_IDS) {
      await expect(testPage.getByTestId(`background-work-state-${shellID}`)).toHaveText(
        "Completed",
      );
    }
  });
});
