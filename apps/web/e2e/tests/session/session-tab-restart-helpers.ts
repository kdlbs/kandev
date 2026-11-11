import { expect, type Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { watchWs } from "../../helpers/causal-waits";
import { attachGatewayTrafficCapture } from "../../helpers/ws-traffic";

/** Creates an idle mock session and observes gateway traffic before navigation. */
export async function createRestartScenario(page: Page, api: ApiClient, seed: SeedData) {
  const wire = watchWs(page);
  const traffic = attachGatewayTrafficCapture(page);
  const task = await api.createTaskWithAgent(
    seed.workspaceId,
    "Session restart",
    seed.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seed.workflowId,
      workflow_step_id: seed.startStepId,
      repository_ids: [seed.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("Restart fixture has no session");
  const sessionId = task.session_id;
  await expect
    .poll(
      async () => {
        const { sessions } = await api.listTaskSessions(task.id);
        return sessions.find((s) => s.id === sessionId)?.state;
      },
      { message: "Waiting for the restart fixture's first turn", timeout: 30_000 },
    )
    .toBe("WAITING_FOR_INPUT");
  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible();
  return { task, sessionId, session, wire, traffic };
}

/** Checks editable-draft retention and rejects button and keyboard prompt admission. */
export async function assertRestartSubmissionBlocked(
  page: Page,
  fixture: Awaited<ReturnType<typeof createRestartScenario>>,
  draftText: string,
) {
  await expect(page.getByTestId("session-restart-status")).toHaveText("Restarting session...");
  const editor = fixture.session.activeChat().locator(".tiptap.ProseMirror:visible").first();
  await expect(editor).toHaveAttribute("contenteditable", "true");
  await editor.fill(draftText);
  const send = fixture.session.activeChat().getByTestId("submit-message-button");
  await expect(send).toBeDisabled();
  await send.evaluate((button: HTMLButtonElement) => button.click());
  await fixture.session.submitMessageWithKeyboard(editor);
  await expect(editor).toContainText(draftText);
  expect(
    fixture.traffic.frames.filter(
      (frame) =>
        frame.direction === "sent" &&
        ["message.add", "message.queue.add"].includes(frame.action ?? ""),
    ),
  ).toEqual([]);
}

/** Captures conversation and workspace identities used to detect replacement or data loss. */
export async function captureRestartBaseline(api: ApiClient, taskId: string, sessionId: string) {
  const sessions = (await api.listTaskSessions(taskId)).sessions;
  const target = sessions.find((s) => s.id === sessionId);
  expect(target).toBeDefined();
  const messages = (await api.listSessionMessages(sessionId)).messages;
  const environment = await api.getTaskEnvironment(taskId);
  expect(environment?.id).toBeTruthy();
  expect(environment?.workspace_path).toBeTruthy();
  return { sessions, target: target!, messages, environment };
}

/** Waits for the resumed agent to become idle before checking retained data. */
export async function assertRestartRetained(
  api: ApiClient,
  taskId: string,
  sessionId: string,
  before: Awaited<ReturnType<typeof captureRestartBaseline>>,
) {
  await expect
    .poll(
      async () => {
        const { sessions } = await api.listTaskSessions(taskId);
        return sessions.find((s) => s.id === sessionId)?.state;
      },
      { message: "Waiting for the restarted agent to become ready", timeout: 30_000 },
    )
    .toBe("WAITING_FOR_INPUT");
  await assertRestartDataRetained(api, taskId, sessionId, before);
}

/** Checks retention without requiring readiness, including after a failed Resume. */
export async function assertRestartDataRetained(
  api: ApiClient,
  taskId: string,
  sessionId: string,
  before: Awaited<ReturnType<typeof captureRestartBaseline>>,
) {
  const after = await captureRestartBaseline(api, taskId, sessionId);
  expect(after.sessions.map((s) => s.id).sort()).toEqual(before.sessions.map((s) => s.id).sort());
  const promotedSibling = before.target.is_primary
    ? before.sessions.find(
        (s) => s.id !== sessionId && ["RUNNING", "STARTING", "WAITING_FOR_INPUT"].includes(s.state),
      )
    : undefined;
  expect(after.target.is_primary).toBe(promotedSibling ? false : before.target.is_primary);
  expect(after.target.worktree_path).toBe(before.target.worktree_path);
  expect(after.environment?.id).toBe(before.environment?.id);
  expect(after.environment?.workspace_path).toBe(before.environment?.workspace_path);
  expect(after.environment?.repos?.map((repo) => [repo.repository_id, repo.worktree_path])).toEqual(
    before.environment?.repos?.map((repo) => [repo.repository_id, repo.worktree_path]),
  );
  const retainedIds = new Set(after.messages.map((m) => m.id));
  for (const message of before.messages) expect(retainedIds.has(message.id)).toBe(true);
  expect(after.messages.filter((m) => m.author_type === "user").map((m) => m.id)).toEqual(
    before.messages.filter((m) => m.author_type === "user").map((m) => m.id),
  );
  assertSiblingsRetained(before, after, sessionId, promotedSibling?.id);
}

/** Allows normal primary promotion while preserving sibling state and execution identity. */
function assertSiblingsRetained(
  before: Awaited<ReturnType<typeof captureRestartBaseline>>,
  after: Awaited<ReturnType<typeof captureRestartBaseline>>,
  sessionId: string,
  promotedSiblingId: string | undefined,
) {
  for (const sibling of before.sessions.filter((s) => s.id !== sessionId)) {
    const retainedSibling = after.sessions.find((s) => s.id === sibling.id);
    expect(retainedSibling?.state).toBe(sibling.state);
    expect(retainedSibling?.agent_execution_id).toBe(sibling.agent_execution_id);
    expect(retainedSibling?.is_primary).toBe(
      promotedSiblingId === sibling.id ? true : sibling.is_primary,
    );
  }
}
