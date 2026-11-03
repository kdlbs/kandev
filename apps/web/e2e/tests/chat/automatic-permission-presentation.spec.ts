import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import { dwell } from "../../helpers/causal-waits";
import {
  createAutomaticPermissionGate,
  createAutomaticPermissionProfile,
  installPermissionPresentationObserver,
  readPermissionPresentationObservation,
} from "../../helpers/automatic-permission-presentation";
import { SessionPage } from "../../pages/session-page";

const COMMITTED_LINE = /git-commit-permission: committed [0-9a-f]{7,40}/;

async function readPermissionHistory(apiClient: ApiClient, sessionId: string) {
  const { messages } = await apiClient.listSessionMessages(sessionId);
  return messages.find((message) => message.type === "permission_request")?.metadata;
}

async function seedWaitingPermissionTask(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  metadata: Record<string, unknown>,
  toolCallId?: string,
) {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Seeded permission presentation",
    seedData.agentProfileId,
    {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
      start_agent: false,
    },
  );
  const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
    state: "WAITING_FOR_INPUT",
    agentProfileId: seedData.agentProfileId,
    repositoryId: seedData.repositoryId,
  });
  const now = new Date().toISOString();
  if (toolCallId) {
    await apiClient.seedSessionMessage(sessionId, {
      type: "tool_call",
      content: "Run git status",
      metadata: { tool_call_id: toolCallId, title: "Run git status", status: "complete" },
      newTurn: true,
      turnStartedAt: now,
      turnCompletedAt: now,
    });
  }
  await apiClient.seedSessionMessage(sessionId, {
    type: "permission_request",
    content: "Run git status",
    metadata,
    newTurn: !toolCallId,
    turnStartedAt: now,
    turnCompletedAt: now,
  });
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  return session;
}

test.describe("Automatic permission presentation", () => {
  test.describe.configure({ retries: 1 });

  test.beforeAll(async ({ backend }) => {
    await backend.restart({ AGENTCTL_AUTO_APPROVE_PERMISSIONS: "false" });
  });
  test.afterAll(async ({ backend }) => {
    await backend.restart({ AGENTCTL_AUTO_APPROVE_PERMISSIONS: "true" });
  });

  test("automatic approval never presents an input prompt", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    const profile = await createAutomaticPermissionProfile(apiClient, seedData.agentProfileId);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Automatic permission presentation",
      profile.id,
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        start_agent: false,
      },
    );
    const prepared = await apiClient.ensureTaskSession(task.id);
    if (!prepared.session_id) throw new Error("ensureTaskSession did not return a session_id");

    const gate = await createAutomaticPermissionGate(testPage, prepared.session_id);
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 60_000 });

    await installPermissionPresentationObserver(testPage);
    gate.arm();
    try {
      await session.sendMessage("/e2e:git-commit-permission");
      await expect.poll(() => gate.heldCount(), { timeout: 30_000 }).toBeGreaterThan(0);

      await expect(session.permissionActionRows()).toHaveCount(0);
      await expect(session.chat).not.toContainText(COMMITTED_LINE);
      await dwell(
        testPage,
        150,
        "negative-assertion",
        "permission completion frames are held while the automatic row is observed",
      );
      await testPage.evaluate(() => window.dispatchEvent(new Event("focus")));
      await expect.poll(() => gate.snapshotHeldCount(), { timeout: 15_000 }).toBeGreaterThan(0);
      await expect(session.permissionActionRows()).toHaveCount(0);
      await expect(session.chat).not.toContainText(COMMITTED_LINE);
      const observation = await readPermissionPresentationObservation(testPage);
      expect(observation).toMatchObject({
        actionRows: 0,
        approveButtons: 0,
        permissionMessages: 0,
        amber: 0,
      });
    } finally {
      await gate.release();
    }

    await session.waitForChatIdle({ timeout: 60_000 });
    await expect(session.chat).toContainText(COMMITTED_LINE);
    await expect(session.permissionActionRows()).toHaveCount(0);
    await prCapture.screenshot("desktop-automatic-permission-absent", {
      caption: "Automatic approval keeps the ordinary completed tool row with no approval prompt.",
    });

    const history = await readPermissionHistory(apiClient, prepared.session_id);
    expect(history).toMatchObject({
      permission_presentation: "automatic",
      status: "approved",
      permission_resolution: {
        option_id: "allow",
        option_kind: "allow_once",
        source: "auto_approve",
        result: "accepted",
      },
    });

    await testPage.reload();
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 60_000 });
    await expect(session.permissionActionRows()).toHaveCount(0);
    await expect(session.chat).toContainText(COMMITTED_LINE);
    expect(await readPermissionHistory(apiClient, prepared.session_id)).toEqual(history);
  });

  test("an interactive fallback stays answerable", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    const session = await seedWaitingPermissionTask(testPage, apiClient, seedData, {
      request_id: "req-fallback",
      pending_id: "pend-fallback",
      tool_call_id: "tool-fallback",
      action_type: "command",
      action_details: { command: "git status" },
      status: "pending",
      permission_presentation: "interactive",
      options: [{ option_id: "allow-once", name: "Allow once", kind: "allow_once" }],
    });

    const message = session.activeChat().getByTestId("permission-request-message");
    await expect(message).toHaveAttribute("data-permission-presentation", "interactive");
    await expect(session.permissionActionRows()).toHaveCount(1);
    await expect(session.permissionApproveButtons()).toHaveCount(1);
    await prCapture.screenshot("desktop-interactive-permission-fallback", {
      caption: "Interactive fallback keeps its inline permission choices.",
    });
  });

  test("an unavailable decision keeps non-actionable feedback", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    const session = await seedWaitingPermissionTask(
      testPage,
      apiClient,
      seedData,
      {
        request_id: "req-unavailable",
        pending_id: "pend-unavailable",
        tool_call_id: "tool-unavailable",
        action_type: "command",
        action_details: { command: "git status" },
        status: "pending",
        permission_presentation: "unavailable",
        options: [{ option_id: "allow-once", name: "Allow once", kind: "allow_once" }],
      },
      "tool-unavailable",
    );

    await expect(session.activeChat().getByTestId("permission-unavailable-feedback")).toBeVisible();
    await expect(session.activeChat().getByTestId("permission-request-message")).toHaveCount(0);
    await expect(session.permissionActionRows()).toHaveCount(0);
    await prCapture.screenshot("desktop-attached-permission-unavailable", {
      caption: "An unavailable attached decision shows non-actionable feedback.",
    });
  });

  test("a real attended request completes after a click", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Attended permission presentation",
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

    await expect(session.permissionApproveButtons()).toHaveCount(1, { timeout: 30_000 });
    await expect(session.chat).not.toContainText(COMMITTED_LINE);
    await session.permissionApproveButtons().first().click();
    await session.waitForChatIdle({ timeout: 60_000 });
    await expect(session.chat).toContainText(COMMITTED_LINE);
    await expect(session.permissionActionRows()).toHaveCount(0);
  });
});
