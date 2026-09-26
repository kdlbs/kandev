import { expect } from "@playwright/test";
import type { ApiClient } from "./api-client";

export const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];
export const FIRST_PROMPT_MARKER = "prompt-history-auto-load-first-7f1c";
export const SECOND_PROMPT_MARKER = "prompt-history-auto-load-second-2b9e";
export const MIDDLE_PROMPT_MARKER = "prompt-history-unloaded-middle-8c42";

type SeedContext = {
  workspaceId: string;
  agentProfileId: string;
  workflowId: string;
  startStepId: string;
  repositoryId: string;
};

/**
 * Seeds a 121-prompt session: the boot description becomes user prompt #1,
 * the SECOND_PROMPT_MARKER gets the earliest seed timestamp (absolute ordinal
 * #2), and 119 more user prompts follow at least 2ms apart with one agent
 * response between each prompt. The response rows make a middle prompt absent
 * from the transcript's newest-message page while it remains in the history
 * panel's newest-prompt page. The pre-seed count (exactly one user message)
 * pins the marker's absolute ordinal.
 */
export async function seedLongPromptHistory(
  apiClient: ApiClient,
  seed: SeedContext,
): Promise<string> {
  const task = await apiClient.createTaskWithAgent(
    seed.workspaceId,
    "prompt-history-auto-load",
    seed.agentProfileId,
    {
      description: FIRST_PROMPT_MARKER,
      workflow_id: seed.workflowId,
      workflow_step_id: seed.startStepId,
      repository_ids: [seed.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("auto-load task did not create a session");
  const sessionId = task.session_id;

  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(task.id);
        return DONE_STATES.includes(sessions[0]?.state ?? "");
      },
      { timeout: 60_000, message: "Waiting for the boot turn to settle" },
    )
    .toBe(true);

  const { messages } = await apiClient.listSessionMessages(sessionId);
  const preSeedUserMessages = messages.filter((m) => m.author_type === "user");
  if (preSeedUserMessages.length !== 1) {
    throw new Error(`pre-seed user messages = ${preSeedUserMessages.length}, want exactly 1`);
  }

  // seedBase = max(created_at) + 1 second; the marker takes the earliest seed
  // timestamp so its absolute ordinal is pinned at #2.
  const maxCreatedAt = messages.reduce(
    (latest, m) => (Date.parse(m.created_at) > latest ? Date.parse(m.created_at) : latest),
    0,
  );
  const seedBaseMs = maxCreatedAt + 1000;
  // RFC3339 with a zero-padded 6-digit (microsecond) fraction: pad the
  // millisecond fraction instead of replacing it, so every seed is distinct.
  const aligned = (offsetMs: number): string =>
    new Date(seedBaseMs + offsetMs)
      .toISOString()
      .replace(/\.(\d+)Z$/, (_match, fraction: string) => `.${fraction.padEnd(6, "0")}Z`);
  await apiClient.seedSessionMessage(sessionId, {
    type: "message",
    content: SECOND_PROMPT_MARKER,
    authorType: "user",
    createdAt: aligned(0),
  });
  await apiClient.seedSessionMessage(sessionId, {
    type: "message",
    content: "auto-load agent response 0",
    authorType: "agent",
    createdAt: aligned(1),
  });
  for (let i = 1; i < 120; i++) {
    await apiClient.seedSessionMessage(sessionId, {
      type: "message",
      content: i === 59 ? MIDDLE_PROMPT_MARKER : `auto-load filler prompt ${i}`,
      authorType: "user",
      createdAt: aligned(i * 2),
    });
    await apiClient.seedSessionMessage(sessionId, {
      type: "message",
      content: `auto-load agent response ${i}`,
      authorType: "agent",
      createdAt: aligned(i * 2 + 1),
    });
  }

  // Refetch and assert the stored user timestamps are strictly increasing
  // (the pre-seed count pins the marker's absolute ordinal).
  const afterSeed = await apiClient.listSessionMessages(sessionId);
  const seedUserTimestamps = afterSeed.messages
    .filter((m) => m.author_type === "user" && !m.content.includes(FIRST_PROMPT_MARKER))
    .map((m) => Date.parse(m.created_at));
  if (seedUserTimestamps.length !== 120) {
    throw new Error(`seeded user messages = ${seedUserTimestamps.length}, want 120`);
  }
  for (let i = 1; i < seedUserTimestamps.length; i++) {
    if (seedUserTimestamps[i] <= seedUserTimestamps[i - 1]) {
      throw new Error(`seed timestamps not strictly increasing at ${i}`);
    }
  }
  return task.id;
}
