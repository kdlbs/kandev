import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";

/**
 * Regression coverage for the Kandev MCP system block (kandev-context.md)
 * that the orchestrator wraps around the FIRST prompt of every task launch.
 *
 * Contract:
 *  - The wrap (<kandev-system>...</kandev-system>) is stored on the user
 *    message row so the chat UI can reveal it under "Show formatted" and
 *    so the audit trail records exactly what the agent saw.
 *  - The visible `content` is the user's original text — the wrap is
 *    stripped server-side via Message.ToAPI.
 *  - The wrap carries the task ID, session ID, and the MCP tools list
 *    (ask_user_question_kandev, create_task_plan_kandev, …) so the agent
 *    can act on the kandev platform without re-discovering its identifiers.
 *  - Wrap is applied ONCE on the first prompt. Follow-up prompts and
 *    resumes do not re-wrap (covered by backend lifecycle tests).
 */

async function waitForUserMessage(
  apiClient: ApiClient,
  sessionId: string,
  timeoutMs = 20_000,
): Promise<{ content: string; raw_content?: string; metadata?: Record<string, unknown> }> {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const { messages } = await apiClient.listSessionMessages(sessionId);
    const user = messages.find((m) => m.author_type === "user");
    if (user) return user;
    await new Promise((r) => setTimeout(r, 250));
  }
  throw new Error(`No user message recorded on session ${sessionId} within ${timeoutMs}ms`);
}

test.describe("Kandev system prompt wrap on first launch", () => {
  test("kanban task records the wrap in raw_content and strips it from content", async ({
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Wrap-on-launch regression",
      seedData.agentProfileId,
      {
        description: 'e2e:message("ack")',
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

    const recorded = await waitForUserMessage(apiClient, task.session_id);

    // Visible bubble text — what the UI shows under the default "formatted" view.
    expect(recorded.content).not.toContain("<kandev-system>");
    expect(recorded.content).toContain('e2e:message("ack")');

    // raw_content carries the wrap. Message.ToAPI only sets raw_content when
    // hidden system blocks are present, so its presence is itself a signal
    // that the wrap reached the DB.
    const raw = recorded.raw_content ?? "";
    expect(raw).toContain("<kandev-system>");
    expect(raw).toContain("</kandev-system>");
    // Exactly-once: if any future call site (wsAddMessage, recordAutoStartMessage,
    // StartCreatedSession) lost its idempotency guard and double-wrapped, the
    // backend unit test catches it but the agent-facing boundary needs its own
    // assertion too — nested <kandev-system> blocks break the strip regex.
    expect((raw.match(/<kandev-system>/g) ?? []).length).toBe(1);
    expect((raw.match(/<\/kandev-system>/g) ?? []).length).toBe(1);
    expect(raw).toContain(`Kandev Task ID: ${task.id}`);
    expect(raw).toContain(`Kandev Session ID: ${task.session_id}`);
    // Guard one representative MCP tool from kandev-context.md — if the
    // template drifts the test will fail loudly rather than silently passing
    // an empty wrap.
    expect(raw).toContain("ask_user_question_kandev");
    expect(raw).toContain("create_task_plan_kandev");
    // The user's text must survive inside the wrapped form.
    expect(raw).toContain('e2e:message("ack")');

    // metadata.has_hidden_prompts drives the UI's "Show formatted" toggle.
    expect(recorded.metadata?.has_hidden_prompts).toBe(true);
  });
});
