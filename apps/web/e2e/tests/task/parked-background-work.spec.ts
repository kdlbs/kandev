/**
 * E2E coverage for the parked-on-background-work affordance (spec
 * docs/specs/disambiguate-waiting/spec.md, task-09). The projection itself
 * (probe port, sampling loop, revision ordering) is covered by Go unit tests
 * in internal/orchestrator/parked_projection_test.go; these specs prove the
 * whole pipeline end-to-end through a real backend + WebSocket + browser:
 * BackgroundProbe answers -> parked projection -> session.activity_changed /
 * task.updated -> store -> rendered icon, using the mock agent's
 * `/parked-fixture` command plus the KANDEV_E2E_MOCK-gated
 * POST /api/v1/_test/background-probe test harness route to script the
 * probe's answers deterministically.
 *
 * `/parked-fixture [settleDelay]` registers a shell-kind detached background
 * launch (run_in_background:true — the one condition the parked projection's
 * attestation term actually recognises, see cmd/mock-agent/handler.go's
 * emitParkedFixture) and delays the foreground turn by settleDelay first,
 * giving the test a deterministic window to read the freshly-created
 * session's ID and script its probe sequence *before* the turn settles and
 * triggers the synchronous first probe sample (spec D2). The attestation
 * persists until the session's next turn starts (spec D3), so every test
 * below scripts the probe to hold "live" indefinitely (ScriptedBackgroundProbe
 * holds at a sequence's last entry once exhausted) rather than racing a
 * fixed-length sequence against its own setup latency — a settle-to-clear
 * transition is driven by explicitly re-scripting the probe afterward.
 *
 * The shell-kind recogniser is registered only for agent ID "claude-acp"
 * (internal/agentctl/server/adapter/transport/acp/background_launch_recognizer.go)
 * — the default e2e mock agent's ACP session reports agent_type "mock-agent",
 * which has no registered recogniser at all, so the default seedData agent
 * profile can never attest. The suite opts into KANDEV_MOCK_PROVIDERS=claude-acp
 * (same mechanism the office-routing-* specs use) so the mock binary is also
 * registered under a "claude-acp" profile, then uses that profile for every
 * task created below.
 */
import { expect, test } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

const PARKED_PROBE_INTERVAL = "1s";
const PARKED_FIXTURE_SCRIPT = "/parked-fixture 3s";

async function waitForSessionState(
  apiClient: ApiClient,
  taskId: string,
  state: string,
  message: string,
): Promise<void> {
  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(taskId);
        return sessions[0]?.state ?? "";
      },
      { message, timeout: 30_000 },
    )
    .toBe(state);
}

/** First session row to appear for the task, regardless of state — used to
 * script the probe before the foreground turn settles. */
async function waitForFirstSessionId(apiClient: ApiClient, taskId: string): Promise<string> {
  let sessionId = "";
  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(taskId);
        sessionId = sessions[0]?.id ?? "";
        return sessionId;
      },
      { message: "session row should be created", timeout: 15_000 },
    )
    .not.toBe("");
  return sessionId;
}

/** Creates a task whose foreground turn registers a shell-kind detached
 * launch after `settleDelay`, scripts its probe to hold "live" indefinitely
 * before that settle can happen, then waits for the settle itself. */
async function createParkedTask(
  apiClient: ApiClient,
  seedData: { workspaceId: string; repositoryId: string; workflowId: string; startStepId: string },
  agentProfileId: string,
  title: string,
): Promise<{ taskId: string; sessionId: string }> {
  const task = await apiClient.createTaskWithAgent(seedData.workspaceId, title, agentProfileId, {
    description: PARKED_FIXTURE_SCRIPT,
    repository_ids: [seedData.repositoryId],
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });

  const sessionId = await waitForFirstSessionId(apiClient, task.id);
  await apiClient.scriptBackgroundProbe(sessionId, ["live"]);

  await waitForSessionState(
    apiClient,
    task.id,
    "WAITING_FOR_INPUT",
    `${title}'s foreground turn should settle`,
  );

  return { taskId: task.id, sessionId };
}

test.describe("Parked on background work", () => {
  let claudeAcpProfileId: string;

  test.beforeAll(async ({ backend, apiClient }) => {
    await backend.restart({
      KANDEV_PARKED_PROBE_INTERVAL: PARKED_PROBE_INTERVAL,
      KANDEV_MOCK_PROVIDERS: "claude-acp",
    });
    const { agents } = await apiClient.listAgents();
    const claudeAgent = agents.find((agent) => agent.name === "claude-acp");
    const profileId = claudeAgent?.profiles[0]?.id;
    if (!profileId) {
      throw new Error(
        "E2E seed has no claude-acp mock profile after KANDEV_MOCK_PROVIDERS restart",
      );
    }
    claudeAcpProfileId = profileId;
  });

  test.afterAll(async ({ backend }) => {
    await backend.restart();
  });

  test("AC-73/AC-62: live samples show the affordance in the sidebar, settle clears it via WS", async ({
    apiClient,
    seedData,
    testPage,
  }) => {
    test.setTimeout(120_000);

    // Mount the sidebar on a different task first so the client is live
    // before the parked task's own WS traffic starts (see
    // sidebar-settled-spinner.spec.ts for why this ordering matters).
    const navTask = await apiClient.createTask(seedData.workspaceId, "Nav Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await testPage.goto(`/t/${navTask.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.sidebar).toBeVisible({ timeout: 10_000 });

    const { sessionId } = await createParkedTask(
      apiClient,
      seedData,
      claudeAcpProfileId,
      "Parked Turn",
    );

    const parkedRow = session.sidebarTaskItem("Parked Turn");
    await expect(parkedRow).toBeVisible({ timeout: 15_000 });

    // Core regression (AC-73): the synchronous settle sample already reads
    // "live" (scripted before settle), so the affordance must render at
    // settle and hold — the script never advances past "live" until the
    // test flips it below.
    await expect(parkedRow.getByTestId("task-state-background-running")).toBeVisible({
      timeout: 10_000,
    });
    await expect(parkedRow.getByTestId("task-state-turn-finished")).toHaveCount(0);

    // AC-62: the next periodic sample (KANDEV_PARKED_PROBE_INTERVAL=1s) reads
    // "settled" and clears the affordance; the row updates from the live
    // task.updated stream — no reload anywhere above.
    await apiClient.scriptBackgroundProbe(sessionId, ["settled"]);
    await expect(parkedRow.getByTestId("task-state-background-running")).toHaveCount(0, {
      timeout: 10_000,
    });
  });

  test("AC-68: leaving WAITING_FOR_INPUT clears the affordance immediately, without a new probe sample", async ({
    apiClient,
    seedData,
    testPage,
  }) => {
    test.setTimeout(120_000);

    const { taskId } = await createParkedTask(
      apiClient,
      seedData,
      claudeAcpProfileId,
      "Parked Resume Turn",
    );

    await testPage.goto(`/t/${taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.sidebar).toBeVisible({ timeout: 10_000 });

    const parkedRow = session.sidebarTaskItem("Parked Resume Turn");
    await expect(parkedRow.getByTestId("task-state-background-running")).toBeVisible({
      timeout: 15_000,
    });

    // The probe is still scripted to hold "live" forever — AC-68 must clear
    // on the state transition alone, never by waiting for a different probe
    // answer.
    await session.sendMessage("keep going");

    await expect(parkedRow.getByTestId("task-state-background-running")).toHaveCount(0, {
      timeout: 10_000,
    });
  });
});
