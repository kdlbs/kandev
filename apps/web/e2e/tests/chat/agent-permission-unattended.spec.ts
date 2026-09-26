import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

/**
 * Both directions of REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-005.
 *
 * The `/e2e:git-commit-permission` mock-agent scenario asks for permission to
 * run a state-changing Git command and creates the commit only when the request
 * is granted. Acceptance is the resolved commit object, never a displayed mode,
 * a log line, or the absence of an error message: the field report that
 * motivated this package had all three of those looking correct while the
 * commands were still refused.
 *
 * The two runs differ only by agent profile. A mechanism verified in its
 * success case alone says nothing about the case it exists for, so the
 * attended direction is as load-bearing as the unattended one.
 */
test.describe("Unattended permission for a state-changing Git command", () => {
  test.describe.configure({ retries: 1 });

  // Most specs run with blanket auto-approval on, which would make both
  // directions look identical. Turn it off so the profile is the only
  // difference between the two runs.
  test.beforeAll(async ({ backend }) => {
    await backend.restart({ AGENTCTL_AUTO_APPROVE_PERMISSIONS: "false" });
  });
  test.afterAll(async ({ backend }) => {
    await backend.restart({ AGENTCTL_AUTO_APPROVE_PERMISSIONS: "true" });
  });

  test("an unattended profile commits with nobody answering", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const agents = await apiClient.listAgents();
    const mockAgent = agents.agents.find((a) =>
      a.profiles?.some((p) => p.id === seedData.agentProfileId),
    );
    if (!mockAgent) throw new Error("could not resolve the seeded agent for profile creation");

    const unattended = await apiClient.createAgentProfile(mockAgent.id, "Unattended permissions", {
      model: "mock-model",
      auto_approve: true,
    });

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Unattended git commit",
      unattended.id,
      {
        description: "/e2e:git-commit-permission",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 60_000 });

    // Nobody answered anything: no prompt was ever surfaced.
    await expect(session.permissionActionRows()).toHaveCount(0);
    await expect(session.chat).toContainText(COMMITTED_LINE);

    const history = await readAutoApprovalHistory(apiClient, task.session_id);
    expect(history).toMatchObject({
      status: "approved",
      permission_decision: {
        option_id: "allow",
        option_kind: "allow_once",
        source: "auto_approve",
      },
    });

    // The acceptance evidence is the commit object. Assert on the transcript
    // first so the locator auto-waits for the scenario's closing line instead
    // of racing a one-shot read against it.
    expect(await readCommittedSha(session)).toMatch(/^[0-9a-f]{7,40}$/);

    await testPage.reload();
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 60_000 });
    expect(await readAutoApprovalHistory(apiClient, task.session_id)).toEqual(history);
    await expect(session.chat).toContainText(COMMITTED_LINE);
  });

  test("the default profile holds the commit until a person answers", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Attended git commit",
      seedData.agentProfileId,
      {
        description: "/e2e:git-commit-permission",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // The call must stop here rather than proceeding on its own.
    await expect(session.permissionApproveButtons()).toHaveCount(1, { timeout: 30_000 });
    await expect(session.chat).not.toContainText(COMMITTED_LINE);

    await session.permissionApproveButtons().first().click();
    await session.waitForChatIdle({ timeout: 60_000 });

    await expect(session.chat).toContainText(COMMITTED_LINE);
    expect(await readCommittedSha(session)).toMatch(/^[0-9a-f]{7,40}$/);
  });
});

/** The closing line `/e2e:git-commit-permission` emits after it commits. */
const COMMITTED_LINE = /git-commit-permission: committed [0-9a-f]{7,40}/;

/**
 * Reads the SHA the scenario reports after it committed. Returns the empty
 * string when the scenario refused, so a caller asserting a SHA fails with the
 * refusal rather than with a timeout.
 */
async function readCommittedSha(session: SessionPage): Promise<string> {
  const transcript = (await session.chat.textContent()) ?? "";
  const match = transcript.match(/git-commit-permission: committed ([0-9a-f]{7,40})/);
  return match ? match[1] : "";
}

async function readAutoApprovalHistory(apiClient: ApiClient, sessionId: string): Promise<unknown> {
  const { messages } = await apiClient.listSessionMessages(sessionId);
  const permission = messages.find((message) => message.type === "permission_request");
  return permission?.metadata;
}
