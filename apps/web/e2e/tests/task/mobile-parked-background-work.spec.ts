/**
 * Mobile coverage for the parked-on-background-work affordance in the session
 * switcher (spec docs/specs/disambiguate-waiting/spec.md, task-09, AC-51/AC-34).
 *
 * The desktop chat toolbar's session switcher (components/task/sessions-dropdown.tsx)
 * is unreachable from the primary Dockview session view: every caller of the
 * shared chat toolbar (dockview-panel-content.tsx, dockview-shared.tsx,
 * quick-chat-content.tsx, preview-session-tabs.tsx, passthrough-chat-composer.tsx)
 * sets `hideSessionsDropdown`, and the desktop chat toolbar itself also hides it
 * below the `md` breakpoint. The session switcher row this AC describes is the
 * mobile picker sheet (components/task/mobile/mobile-sessions-section.tsx),
 * reached via `mobile-sessions-pill` — hence this spec lives in its own
 * `mobile-*.spec.ts` file so Playwright's `mobile-.*\.spec\.ts` project match
 * runs it under the Pixel 5 device profile (see e2e/playwright.config.ts).
 *
 * See parked-background-work.spec.ts for the desktop sidebar/state-transition
 * coverage and the shared mechanics comment (probe scripting, shell-kind
 * detached-launch fixture, KANDEV_MOCK_PROVIDERS=claude-acp).
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

test.describe("mobile: parked on background work", () => {
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

  test("AC-51/AC-34: session switcher row reflects the parked reading, pending input still wins", async ({
    apiClient,
    seedData,
    testPage,
  }) => {
    test.setTimeout(120_000);

    const { taskId, sessionId } = await createParkedTask(
      apiClient,
      seedData,
      claudeAcpProfileId,
      "Parked Switcher Turn",
    );

    await testPage.goto(`/t/${taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const layout = testPage.locator("[data-testid='mobile-task-layout']:visible");
    const pill = layout.getByTestId("mobile-sessions-pill");
    await expect(pill).toBeVisible({ timeout: 30_000 });
    await pill.tap();

    const sheet = testPage.getByRole("dialog", { name: "Sessions" });
    await expect(sheet).toBeVisible({ timeout: 5_000 });
    const row = sheet.getByTestId(`mobile-session-row-${sessionId}`);
    await expect(row).toBeVisible({ timeout: 10_000 });
    await expect(row.locator(".tabler-icon-loader-2")).toBeVisible({ timeout: 10_000 });

    // AC-34: a concurrent pending clarification outranks the parked reading
    // — the row must switch to the message-question icon, not stay on the
    // background spinner, even though the session is still scripted "live".
    await apiClient.seedSessionMessage(sessionId, {
      type: "clarification_request",
      metadata: { status: "pending" },
    });

    await expect(row.locator(".tabler-icon-message-question")).toBeVisible({ timeout: 10_000 });
    await expect(row.locator(".tabler-icon-loader-2")).toHaveCount(0);
  });
});
